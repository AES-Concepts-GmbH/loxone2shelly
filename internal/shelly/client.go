package shelly

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/models"
)

// Client is the Shelly Gen1/Gen2+ HTTP/RPC client
type Client struct {
	devices      map[string]*DeviceInfo // key = config ID
	mu           sync.RWMutex
	httpClient   *http.Client
	eventChan    chan Event
	stopChan     chan struct{}
	lastStates   map[string]interface{}
	offlineState map[string]bool // tracks known-offline devices to reduce log noise
}

// DeviceInfo holds configuration for a single Shelly device
type DeviceInfo struct {
	ConfigID   string
	Name       string
	IP         string
	Generation int
}

// Event represents a state change event from a Shelly device
type Event struct {
	DeviceID string
	Type     string      // "switch", "light", "input", etc
	Data     interface{}
}

// NewClient creates a new Shelly client
func NewClient() *Client {
	return &Client{
		devices: make(map[string]*DeviceInfo),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		eventChan:    make(chan Event, 100),
		stopChan:     make(chan struct{}),
		lastStates:   make(map[string]interface{}),
		offlineState: make(map[string]bool),
	}
}

// AddDevice adds a Shelly device to the client
func (c *Client) AddDevice(id, name, ip string, gen int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.devices[id] = &DeviceInfo{
		ConfigID:   id,
		Name:       name,
		IP:         ip,
		Generation: gen,
	}
	log.Info().Str("id", id).Str("name", name).Str("ip", ip).Int("gen", gen).Msg("Shelly device added")
}

// RemoveDevice removes a Shelly device from the client
func (c *Client) RemoveDevice(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.devices, id)
	delete(c.lastStates, id)
	log.Info().Str("id", id).Msg("Shelly device removed")
}

// IsConfigured returns true if at least one device is configured
func (c *Client) IsConfigured() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.devices) > 0
}

// GetDevices queries all configured Shelly devices in parallel and returns them as models.Device
func (c *Client) GetDevices() []*models.Device {
	c.mu.RLock()
	devicesCopy := make([]*DeviceInfo, 0, len(c.devices))
	for _, d := range c.devices {
		cp := *d
		devicesCopy = append(devicesCopy, &cp)
	}
	c.mu.RUnlock()

	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]*models.Device, 0, len(devicesCopy))

	for _, dev := range devicesCopy {
		wg.Add(1)
		go func(d *DeviceInfo) {
			defer wg.Done()
			device := c.queryDevice(d)
			mu.Lock()
			results = append(results, device)
			mu.Unlock()
		}(dev)
	}

	wg.Wait()
	return results
}

// GetDevice queries a single Shelly device by config ID
func (c *Client) GetDevice(id string) (*models.Device, error) {
	c.mu.RLock()
	dev, ok := c.devices[id]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("device not found: %s", id)
	}

	return c.queryDevice(dev), nil
}

// SetDeviceState sends a command to a Shelly device
func (c *Client) SetDeviceState(id string, cmd models.DeviceCommand) error {
	c.mu.RLock()
	dev, ok := c.devices[id]
	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf("device not found: %s", id)
	}

	if dev.Generation >= 2 {
		return c.setDeviceStateGen2(dev, cmd)
	}
	return c.setDeviceStateGen1(dev, cmd)
}

// GetInputs queries inputs (buttons/switches) from all configured devices
func (c *Client) GetInputs() []*models.Input {
	c.mu.RLock()
	devicesCopy := make([]*DeviceInfo, 0, len(c.devices))
	for _, d := range c.devices {
		cp := *d
		devicesCopy = append(devicesCopy, &cp)
	}
	c.mu.RUnlock()

	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]*models.Input, 0)

	for _, dev := range devicesCopy {
		wg.Add(1)
		go func(d *DeviceInfo) {
			defer wg.Done()
			inputs := c.queryInputs(d)
			mu.Lock()
			results = append(results, inputs...)
			mu.Unlock()
		}(dev)
	}

	wg.Wait()
	return results
}

// Events returns a read-only channel of device events
func (c *Client) Events() <-chan Event {
	return c.eventChan
}

// Close stops the client and releases resources
func (c *Client) Close() {
	close(c.stopChan)
}

// --- Gen2+ RPC helpers ---

func (c *Client) rpcGet(ip, method string) (map[string]interface{}, error) {
	url := fmt.Sprintf("http://%s/rpc/%s", ip, method)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", url, err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON from %s: %w", url, err)
	}
	return result, nil
}

func (c *Client) rpcPost(ip, method string, payload interface{}) (map[string]interface{}, error) {
	url := fmt.Sprintf("http://%s/rpc/%s", ip, method)

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling payload: %w", err)
	}

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("HTTP POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", url, err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON from %s: %w", url, err)
	}
	return result, nil
}

// --- Gen1 HTTP helpers ---

func (c *Client) gen1Get(ip, path string) (map[string]interface{}, error) {
	url := fmt.Sprintf("http://%s/%s", ip, path)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", url, err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON from %s: %w", url, err)
	}
	return result, nil
}

// queryDevice queries a single device and returns a models.Device
func (c *Client) queryDevice(d *DeviceInfo) *models.Device {
	device := &models.Device{
		ID:         d.ConfigID,
		Name:       d.Name,
		IP:         d.IP,
		Generation: d.Generation,
		Online:     false,
	}

	if d.Generation >= 2 {
		c.queryDeviceGen2(d, device)
	} else {
		c.queryDeviceGen1(d, device)
	}

	return device
}

func (c *Client) queryDeviceGen2(d *DeviceInfo, device *models.Device) {
	// Get device info
	info, err := c.rpcGet(d.IP, "Shelly.GetDeviceInfo")
	if err != nil {
		c.mu.Lock()
		wasOffline := c.offlineState[d.ConfigID]
		c.offlineState[d.ConfigID] = true
		c.mu.Unlock()
		if !wasOffline {
			log.Warn().Err(err).Str("id", d.ConfigID).Str("ip", d.IP).Msg("Device unreachable (Gen2)")
		} else {
			log.Debug().Str("id", d.ConfigID).Msg("Device still unreachable (Gen2)")
		}
		return
	}

	// Mark as back online if it was offline
	c.mu.Lock()
	wasOffline := c.offlineState[d.ConfigID]
	c.offlineState[d.ConfigID] = false
	c.mu.Unlock()
	if wasOffline {
		log.Info().Str("id", d.ConfigID).Str("ip", d.IP).Msg("Device back online (Gen2)")
	}

	device.Online = true

	if model, ok := info["model"].(string); ok {
		device.Model = model
	}
	if fw, ok := info["fw_id"].(string); ok {
		device.FirmwareVer = fw
	}
	if app, ok := info["app"].(string); ok {
		device.Type = c.appToDeviceType(app)
	}

	// Get status
	status, err := c.rpcGet(d.IP, "Shelly.GetStatus")
	if err != nil {
		log.Warn().Err(err).Str("id", d.ConfigID).Msg("Failed to get device status (Gen2)")
		return
	}

	c.parseGen2Status(status, device)
}

func (c *Client) queryDeviceGen1(d *DeviceInfo, device *models.Device) {
	status, err := c.gen1Get(d.IP, "status")
	if err != nil {
		c.mu.Lock()
		wasOffline := c.offlineState[d.ConfigID]
		c.offlineState[d.ConfigID] = true
		c.mu.Unlock()
		if !wasOffline {
			log.Warn().Err(err).Str("id", d.ConfigID).Str("ip", d.IP).Msg("Device unreachable (Gen1)")
		} else {
			log.Debug().Str("id", d.ConfigID).Msg("Device still unreachable (Gen1)")
		}
		return
	}

	c.mu.Lock()
	wasOffline := c.offlineState[d.ConfigID]
	c.offlineState[d.ConfigID] = false
	c.mu.Unlock()
	if wasOffline {
		log.Info().Str("id", d.ConfigID).Str("ip", d.IP).Msg("Device back online (Gen1)")
	}

	device.Online = true

	// Try to determine device type from status
	if relays, ok := status["relays"].([]interface{}); ok && len(relays) > 0 {
		if rollers, ok := status["rollers"].([]interface{}); ok && len(rollers) > 0 {
			device.Type = "roller"
			device.Capabilities.SupportsRoller = true
			if roller, ok := rollers[0].(map[string]interface{}); ok {
				if pos, ok := roller["current_pos"].(float64); ok {
					device.State.Position = int(pos)
				}
			}
		} else {
			device.Type = "switch"
			if relay, ok := relays[0].(map[string]interface{}); ok {
				if ison, ok := relay["ison"].(bool); ok {
					device.State.On = ison
				}
				if power, ok := relay["power"].(float64); ok {
					device.State.Power = power
					device.Capabilities.SupportsPower = true
				}
			}
		}
	}

	if lights, ok := status["lights"].([]interface{}); ok && len(lights) > 0 {
		device.Type = "dimmer"
		device.Capabilities.SupportsDimming = true
		if light, ok := lights[0].(map[string]interface{}); ok {
			if ison, ok := light["ison"].(bool); ok {
				device.State.On = ison
			}
			if bri, ok := light["brightness"].(float64); ok {
				device.State.Brightness = bri
			}
		}
	}

	// Firmware
	if update, ok := status["update"].(map[string]interface{}); ok {
		if oldVer, ok := update["old_version"].(string); ok {
			device.FirmwareVer = oldVer
		}
	}
}

// parseGen2Status dynamically parses all Gen2+ status components
func (c *Client) parseGen2Status(status map[string]interface{}, device *models.Device) {
	switchCount := 0
	lightCount := 0
	coverCount := 0

	// Parse all switch:N components
	for i := 0; i < 4; i++ {
		key := fmt.Sprintf("switch:%d", i)
		sw, ok := status[key].(map[string]interface{})
		if !ok {
			continue
		}
		switchCount++

		ch := models.Channel{Index: i, Type: "switch"}
		if output, ok := sw["output"].(bool); ok {
			ch.On = output
		}
		if power, ok := sw["apower"].(float64); ok {
			ch.Power = power
		}
		if voltage, ok := sw["voltage"].(float64); ok {
			ch.Voltage = voltage
		}
		if current, ok := sw["current"].(float64); ok {
			ch.Current = current
		}
		if energy, ok := sw["aenergy"].(map[string]interface{}); ok {
			if total, ok := energy["total"].(float64); ok {
				ch.Energy = total
			}
		}

		// First channel populates primary state
		if i == 0 {
			device.State.On = ch.On
			device.State.Power = ch.Power
			device.State.Voltage = ch.Voltage
			device.State.Current = ch.Current
			device.State.Energy = ch.Energy
			if ch.Power > 0 || ch.Voltage > 0 {
				device.Capabilities.SupportsPower = true
			}
		}

		device.Channels = append(device.Channels, ch)
	}

	if switchCount > 0 && device.Type == "" {
		device.Type = "switch"
	}
	if switchCount > 1 {
		device.Capabilities.MultiChannel = true
		device.Capabilities.ChannelCount = switchCount
	}

	// Parse all light:N components
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("light:%d", i)
		lt, ok := status[key].(map[string]interface{})
		if !ok {
			continue
		}
		lightCount++

		ch := models.Channel{Index: i, Type: "light"}
		if output, ok := lt["output"].(bool); ok {
			ch.On = output
		}
		if bri, ok := lt["brightness"].(float64); ok {
			ch.Brightness = bri
		}
		if power, ok := lt["apower"].(float64); ok {
			ch.Power = power
		}

		if i == 0 {
			device.State.On = ch.On
			device.State.Brightness = ch.Brightness
			device.State.Power = ch.Power
			device.Capabilities.SupportsDimming = true
			if ch.Power > 0 {
				device.Capabilities.SupportsPower = true
			}
		}

		device.Channels = append(device.Channels, ch)
	}

	if lightCount > 0 {
		device.Type = "light"
	}
	if lightCount > 1 {
		device.Capabilities.MultiChannel = true
		device.Capabilities.ChannelCount = lightCount
	}

	// Parse all cover:N components
	for i := 0; i < 2; i++ {
		key := fmt.Sprintf("cover:%d", i)
		cover, ok := status[key].(map[string]interface{})
		if !ok {
			continue
		}
		coverCount++

		ch := models.Channel{Index: i, Type: "cover"}
		if pos, ok := cover["current_pos"].(float64); ok {
			ch.Position = int(pos)
		}
		if power, ok := cover["apower"].(float64); ok {
			ch.Power = power
		}

		if i == 0 {
			device.State.Position = ch.Position
			device.State.Power = ch.Power
			device.Capabilities.SupportsRoller = true
			if ch.Power > 0 {
				device.Capabilities.SupportsPower = true
			}
		}

		device.Channels = append(device.Channels, ch)
	}

	if coverCount > 0 {
		device.Type = "roller"
	}
	if coverCount > 1 {
		device.Capabilities.MultiChannel = true
		device.Capabilities.ChannelCount = coverCount
	}

	// RGB
	if rgb, ok := status["rgb:0"].(map[string]interface{}); ok {
		device.Type = "rgb"
		device.Capabilities.SupportsColor = true
		device.Capabilities.SupportsDimming = true
		if output, ok := rgb["output"].(bool); ok {
			device.State.On = output
		}
		if r, ok := rgb["red"].(float64); ok {
			device.State.Red = int(r)
		}
		if g, ok := rgb["green"].(float64); ok {
			device.State.Green = int(g)
		}
		if b, ok := rgb["blue"].(float64); ok {
			device.State.Blue = int(b)
		}
	}

	// RGBW
	if rgbw, ok := status["rgbw:0"].(map[string]interface{}); ok {
		device.Type = "rgbw"
		device.Capabilities.SupportsColor = true
		device.Capabilities.SupportsDimming = true
		if output, ok := rgbw["output"].(bool); ok {
			device.State.On = output
		}
		if r, ok := rgbw["red"].(float64); ok {
			device.State.Red = int(r)
		}
		if g, ok := rgbw["green"].(float64); ok {
			device.State.Green = int(g)
		}
		if b, ok := rgbw["blue"].(float64); ok {
			device.State.Blue = int(b)
		}
		if w, ok := rgbw["white"].(float64); ok {
			device.State.White = int(w)
		}
	}

	// CCT (tunable white)
	if cct, ok := status["cct:0"].(map[string]interface{}); ok {
		device.Type = "light"
		device.Capabilities.SupportsDimming = true
		device.Capabilities.SupportsColorTemp = true
		if output, ok := cct["output"].(bool); ok {
			device.State.On = output
		}
		if bri, ok := cct["brightness"].(float64); ok {
			device.State.Brightness = bri
		}
		if ct, ok := cct["ct"].(float64); ok {
			device.State.ColorTemp = int(ct)
		}
	}

	// --- Sensor components ---

	// Temperature sensors (built-in and add-on with IDs 100+)
	for i := 0; i < 200; i++ {
		key := fmt.Sprintf("temperature:%d", i)
		temp, ok := status[key].(map[string]interface{})
		if !ok {
			// Skip gaps between 4 and 100 for add-on sensors
			if i > 4 && i < 100 {
				i = 99
			}
			continue
		}
		if tC, ok := temp["tC"].(float64); ok {
			// Only first temperature populates primary state
			if device.State.Temperature == nil {
				device.State.Temperature = &tC
			}
			device.Capabilities.SupportsTemperature = true
		}
	}

	// Humidity sensors (built-in and add-on)
	for i := 0; i < 200; i++ {
		key := fmt.Sprintf("humidity:%d", i)
		hum, ok := status[key].(map[string]interface{})
		if !ok {
			if i > 4 && i < 100 {
				i = 99
			}
			continue
		}
		if rh, ok := hum["rh"].(float64); ok {
			if device.State.Humidity == nil {
				device.State.Humidity = &rh
			}
			device.Capabilities.SupportsHumidity = true
		}
	}

	// Illuminance
	if ill, ok := status["illuminance:0"].(map[string]interface{}); ok {
		if lux, ok := ill["lux"].(float64); ok {
			device.State.Illuminance = &lux
			device.Capabilities.SupportsIlluminance = true
		}
	}

	// Smoke alarm
	if smoke, ok := status["smoke:0"].(map[string]interface{}); ok {
		if alarm, ok := smoke["alarm"].(bool); ok {
			device.State.Smoke = &alarm
			device.Capabilities.SupportsSmoke = true
		}
	}

	// Flood alarm
	if flood, ok := status["flood:0"].(map[string]interface{}); ok {
		if alarm, ok := flood["alarm"].(bool); ok {
			device.State.Flood = &alarm
			device.Capabilities.SupportsFlood = true
		}
	}

	// Battery (devicepower:0)
	if dp, ok := status["devicepower:0"].(map[string]interface{}); ok {
		if bat, ok := dp["battery"].(map[string]interface{}); ok {
			if pct, ok := bat["percent"].(float64); ok {
				device.State.Battery = &pct
				device.Capabilities.SupportsBattery = true
			}
		}
	}

	// --- Energy meter components ---

	// 3-phase energy meter (em:0) - Pro 3EM, 3EM Gen3
	if em, ok := status["em:0"].(map[string]interface{}); ok {
		device.Capabilities.SupportsEnergyMeter = true
		device.Capabilities.SupportsPower = true
		if device.Type == "" || device.Type == "switch" {
			device.Type = "meter"
		}
		if p, ok := em["total_act_power"].(float64); ok {
			device.State.TotalPower = p
			device.State.Power = p
		}
		if p, ok := em["a_act_power"].(float64); ok {
			device.State.PowerA = p
		}
		if p, ok := em["b_act_power"].(float64); ok {
			device.State.PowerB = p
		}
		if p, ok := em["c_act_power"].(float64); ok {
			device.State.PowerC = p
		}
		if v, ok := em["a_voltage"].(float64); ok {
			device.State.VoltageA = v
		}
		if v, ok := em["b_voltage"].(float64); ok {
			device.State.VoltageB = v
		}
		if v, ok := em["c_voltage"].(float64); ok {
			device.State.VoltageC = v
		}
		if a, ok := em["a_current"].(float64); ok {
			device.State.CurrentA = a
		}
		if a, ok := em["b_current"].(float64); ok {
			device.State.CurrentB = a
		}
		if a, ok := em["c_current"].(float64); ok {
			device.State.CurrentC = a
		}
	}

	// 3-phase energy data (emdata:0)
	if emdata, ok := status["emdata:0"].(map[string]interface{}); ok {
		if total, ok := emdata["total_act"].(float64); ok {
			device.State.TotalEnergy = total
			device.State.Energy = total
		}
	}

	// Single-phase energy meters (em1:N) - Pro EM, EM Gen3
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("em1:%d", i)
		em1, ok := status[key].(map[string]interface{})
		if !ok {
			continue
		}
		device.Capabilities.SupportsEnergyMeter = true
		device.Capabilities.SupportsPower = true
		if device.Type == "" || device.Type == "switch" {
			device.Type = "meter"
		}
		if i == 0 {
			if p, ok := em1["act_power"].(float64); ok {
				device.State.Power = p
				device.State.PowerA = p
			}
			if v, ok := em1["voltage"].(float64); ok {
				device.State.Voltage = v
				device.State.VoltageA = v
			}
			if a, ok := em1["current"].(float64); ok {
				device.State.Current = a
				device.State.CurrentA = a
			}
		} else if i == 1 {
			if p, ok := em1["act_power"].(float64); ok {
				device.State.PowerB = p
			}
			if v, ok := em1["voltage"].(float64); ok {
				device.State.VoltageB = v
			}
			if a, ok := em1["current"].(float64); ok {
				device.State.CurrentB = a
			}
		}
	}

	// Single-phase energy data (em1data:N)
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("em1data:%d", i)
		em1data, ok := status[key].(map[string]interface{})
		if !ok {
			continue
		}
		if total, ok := em1data["total_act_energy"].(float64); ok {
			device.State.TotalEnergy += total
			if i == 0 {
				device.State.Energy = total
			}
		}
	}

	// Standalone power meter (pm1:0) - PM Mini Gen3
	if pm1, ok := status["pm1:0"].(map[string]interface{}); ok {
		device.Capabilities.SupportsPower = true
		if device.Type == "" {
			device.Type = "meter"
		}
		if p, ok := pm1["apower"].(float64); ok {
			device.State.Power = p
		}
		if v, ok := pm1["voltage"].(float64); ok {
			device.State.Voltage = v
		}
		if a, ok := pm1["current"].(float64); ok {
			device.State.Current = a
		}
		if energy, ok := pm1["aenergy"].(map[string]interface{}); ok {
			if total, ok := energy["total"].(float64); ok {
				device.State.Energy = total
			}
		}
	}

	// Determine type for sensor-only devices
	if device.Type == "" {
		if device.Capabilities.SupportsSmoke || device.Capabilities.SupportsFlood {
			device.Type = "sensor"
		} else if device.Capabilities.SupportsTemperature || device.Capabilities.SupportsHumidity {
			device.Type = "sensor"
		} else if device.Capabilities.SupportsEnergyMeter {
			device.Type = "meter"
		}
	}
}

// appToDeviceType maps the Gen2 "app" field to a device type
func (c *Client) appToDeviceType(app string) string {
	app = strings.ToLower(app)
	switch {
	case strings.Contains(app, "light"):
		return "light"
	case strings.Contains(app, "dimmer"):
		return "dimmer"
	case strings.Contains(app, "cover"), strings.Contains(app, "roller"):
		return "roller"
	case strings.Contains(app, "rgbw"):
		return "rgbw"
	case strings.Contains(app, "rgb"):
		return "rgb"
	case strings.Contains(app, "plug"):
		return "switch"
	case strings.Contains(app, "switch"), strings.Contains(app, "relay"):
		return "switch"
	case strings.Contains(app, "walldisplay"):
		return "" // has relay + sensors; type determined by parseGen2Status
	case strings.Contains(app, "em"), strings.Contains(app, "pm"):
		return "" // energy meter; type determined by parseGen2Status
	case strings.Contains(app, "i4"), strings.Contains(app, "input"):
		return "switch" // input-only devices
	case strings.Contains(app, "smoke"):
		return "sensor"
	case strings.Contains(app, "flood"):
		return "sensor"
	case strings.Contains(app, "blu"), strings.Contains(app, "sensor"), strings.Contains(app, "ht"):
		return "sensor"
	default:
		return "" // let parseGen2Status determine from components
	}
}

// --- SetDeviceState implementations ---

func (c *Client) setDeviceStateGen2(dev *DeviceInfo, cmd models.DeviceCommand) error {
	// Determine target channel (default 0)
	channelID := 0
	if cmd.Channel != nil {
		channelID = *cmd.Channel
	}

	if cmd.Position != nil {
		payload := map[string]interface{}{
			"id":  channelID,
			"pos": *cmd.Position,
		}
		_, err := c.rpcPost(dev.IP, "Cover.GoToPosition", payload)
		if err != nil {
			return fmt.Errorf("cover command failed for %s: %w", dev.ConfigID, err)
		}
		log.Info().Str("id", dev.ConfigID).Int("channel", channelID).Int("pos", *cmd.Position).Msg("Cover position set (Gen2)")
		return nil
	}

	if cmd.Brightness != nil {
		on := true
		if cmd.On != nil {
			on = *cmd.On
		}
		payload := map[string]interface{}{
			"id":         channelID,
			"on":         on,
			"brightness": int(*cmd.Brightness),
		}
		_, err := c.rpcPost(dev.IP, "Light.Set", payload)
		if err != nil {
			return fmt.Errorf("light command failed for %s: %w", dev.ConfigID, err)
		}
		log.Info().Str("id", dev.ConfigID).Int("channel", channelID).Bool("on", on).Float64("bri", *cmd.Brightness).Msg("Light set (Gen2)")
		return nil
	}

	if cmd.On != nil {
		payload := map[string]interface{}{
			"id": channelID,
			"on": *cmd.On,
		}
		// Try Switch.Set first; for lights fallback to Light.Set
		_, err := c.rpcPost(dev.IP, "Switch.Set", payload)
		if err != nil {
			_, err2 := c.rpcPost(dev.IP, "Light.Set", payload)
			if err2 != nil {
				return fmt.Errorf("switch/light command failed for %s: %w", dev.ConfigID, err)
			}
		}
		log.Info().Str("id", dev.ConfigID).Int("channel", channelID).Bool("on", *cmd.On).Msg("Switch set (Gen2)")
		return nil
	}

	return fmt.Errorf("no actionable command for device %s", dev.ConfigID)
}

func (c *Client) setDeviceStateGen1(dev *DeviceInfo, cmd models.DeviceCommand) error {
	if cmd.Position != nil {
		path := fmt.Sprintf("roller/0?go=to_pos&roller_pos=%d", *cmd.Position)
		_, err := c.gen1Get(dev.IP, path)
		if err != nil {
			return fmt.Errorf("roller command failed for %s: %w", dev.ConfigID, err)
		}
		log.Info().Str("id", dev.ConfigID).Int("pos", *cmd.Position).Msg("Roller position set (Gen1)")
		return nil
	}

	if cmd.Brightness != nil {
		on := "on"
		if cmd.On != nil && !*cmd.On {
			on = "off"
		}
		path := fmt.Sprintf("light/0?turn=%s&brightness=%d", on, int(*cmd.Brightness))
		_, err := c.gen1Get(dev.IP, path)
		if err != nil {
			return fmt.Errorf("dimmer command failed for %s: %w", dev.ConfigID, err)
		}
		log.Info().Str("id", dev.ConfigID).Str("turn", on).Float64("bri", *cmd.Brightness).Msg("Dimmer set (Gen1)")
		return nil
	}

	if cmd.On != nil {
		turn := "off"
		if *cmd.On {
			turn = "on"
		}
		path := fmt.Sprintf("relay/0?turn=%s", turn)
		_, err := c.gen1Get(dev.IP, path)
		if err != nil {
			return fmt.Errorf("relay command failed for %s: %w", dev.ConfigID, err)
		}
		log.Info().Str("id", dev.ConfigID).Str("turn", turn).Msg("Relay set (Gen1)")
		return nil
	}

	return fmt.Errorf("no actionable command for device %s", dev.ConfigID)
}

// queryInputs queries input states from a device
func (c *Client) queryInputs(d *DeviceInfo) []*models.Input {
	var inputs []*models.Input

	if d.Generation >= 2 {
		status, err := c.rpcGet(d.IP, "Shelly.GetStatus")
		if err != nil {
			log.Debug().Str("id", d.ConfigID).Msg("Failed to get inputs (Gen2)")
			return inputs
		}

		// Check for input:0 through input:3 (standard) and input:100+ (add-on)
		for i := 0; i < 200; i++ {
			key := fmt.Sprintf("input:%d", i)
			inp, ok := status[key].(map[string]interface{})
			if !ok {
				if i > 4 && i < 100 {
					i = 99
				}
				continue
			}
			input := &models.Input{
				ID:       fmt.Sprintf("%s_input_%d", d.ConfigID, i),
				Name:     fmt.Sprintf("%s Input %d", d.Name, i),
				DeviceID: d.ConfigID,
			}
			if state, ok := inp["state"].(bool); ok {
				input.State.State = &state
			}
			inputs = append(inputs, input)
		}
	} else {
		// Gen1: query /status and look for inputs
		status, err := c.gen1Get(d.IP, "status")
		if err != nil {
			log.Debug().Str("id", d.ConfigID).Msg("Failed to get inputs (Gen1)")
			return inputs
		}

		if inputsArr, ok := status["inputs"].([]interface{}); ok {
			for i, inp := range inputsArr {
				if inpMap, ok := inp.(map[string]interface{}); ok {
					input := &models.Input{
						ID:       fmt.Sprintf("%s_input_%d", d.ConfigID, i),
						Name:     fmt.Sprintf("%s Input %d", d.Name, i),
						DeviceID: d.ConfigID,
					}
					if inputVal, ok := inpMap["input"].(float64); ok {
						state := inputVal > 0
						input.State.State = &state
					}
					inputs = append(inputs, input)
				}
			}
		}
	}

	return inputs
}
