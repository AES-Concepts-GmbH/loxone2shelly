package shelly

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/models"
)

const (
	// deviceInfoTTL controls how often model/firmware information is refreshed.
	deviceInfoTTL = 10 * time.Minute
	// maxResponseBytes caps a single device response.
	maxResponseBytes = 1 << 20
)

// Client is the Shelly Gen1/Gen2+ HTTP/RPC client. It keeps the last known
// state of every device so API reads do not hit the devices.
type Client struct {
	mu           sync.RWMutex
	devices      map[string]*DeviceInfo     // key = config ID
	cache        map[string]*models.Device  // last known state per device
	inputCache   map[string][]*models.Input // last known inputs per device
	infoCache    map[string]cachedInfo      // Gen2 Shelly.GetDeviceInfo per device
	lastStates   map[string]interface{}     // serialized device state for change detection
	chanStates   map[string]interface{}     // serialized channel state, key "<id>:<channel>"
	offlineState map[string]bool            // tracks known-offline devices to reduce log noise
	httpClient   *http.Client
	eventChan    chan Event
	stopChan     chan struct{}
	stopOnce     sync.Once
	pollInterval time.Duration
	pollReset    chan struct{}
}

type cachedInfo struct {
	model    string
	firmware string
	app      string
	fetched  time.Time
}

// DeviceInfo holds configuration for a single Shelly device
type DeviceInfo struct {
	ConfigID   string
	Name       string
	IP         string
	Generation int
	Username   string
	Password   string
}

// DeviceSpec is the configuration needed to register a device.
type DeviceSpec struct {
	ID         string
	Name       string
	IP         string
	Generation int
	Username   string
	Password   string
}

// Event represents a state change event from a Shelly device
type Event struct {
	DeviceID string
	Type     string // "switch", "light", "roller", "sensor", "meter", "input", ...
	// Channel is set for updates of a single channel of a multi-channel device.
	Channel *int
	// State is the payload forwarded to UI clients (models.DeviceState or a partial map).
	State interface{}
	// Device is the full device snapshot when known (poller / live refresh).
	Device *models.Device
	// Props, when set, are sent to Loxone as-is instead of being derived from Device.
	Props []models.Prop
}

// NewClient creates a new Shelly client
func NewClient() *Client {
	return &Client{
		devices:      make(map[string]*DeviceInfo),
		cache:        make(map[string]*models.Device),
		inputCache:   make(map[string][]*models.Input),
		infoCache:    make(map[string]cachedInfo),
		lastStates:   make(map[string]interface{}),
		chanStates:   make(map[string]interface{}),
		offlineState: make(map[string]bool),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		eventChan:    make(chan Event, 256),
		stopChan:     make(chan struct{}),
		pollInterval: 10 * time.Second,
		pollReset:    make(chan struct{}, 1),
	}
}

// AddDevice adds or replaces a Shelly device.
func (c *Client) AddDevice(spec DeviceSpec) {
	c.mu.Lock()
	c.devices[spec.ID] = &DeviceInfo{
		ConfigID:   spec.ID,
		Name:       spec.Name,
		IP:         spec.IP,
		Generation: spec.Generation,
		Username:   spec.Username,
		Password:   spec.Password,
	}
	c.mu.Unlock()
	log.Info().Str("id", spec.ID).Str("name", spec.Name).Str("ip", spec.IP).Int("gen", spec.Generation).Msg("Shelly device added")
}

// UpdateDevice updates an existing Shelly device. Returns false if unknown.
func (c *Client) UpdateDevice(spec DeviceSpec) bool {
	c.mu.Lock()
	dev, ok := c.devices[spec.ID]
	if !ok {
		c.mu.Unlock()
		return false
	}
	ipChanged := dev.IP != spec.IP || dev.Generation != spec.Generation
	dev.Name = spec.Name
	dev.IP = spec.IP
	dev.Generation = spec.Generation
	dev.Username = spec.Username
	dev.Password = spec.Password
	if ipChanged {
		delete(c.infoCache, spec.ID)
		delete(c.cache, spec.ID)
		delete(c.inputCache, spec.ID)
	}
	c.mu.Unlock()
	log.Info().Str("id", spec.ID).Str("name", spec.Name).Str("ip", spec.IP).Int("gen", spec.Generation).Msg("Shelly device updated")
	return true
}

// RemoveDevice removes a Shelly device from the client
func (c *Client) RemoveDevice(id string) {
	c.mu.Lock()
	delete(c.devices, id)
	delete(c.cache, id)
	delete(c.inputCache, id)
	delete(c.infoCache, id)
	delete(c.lastStates, id)
	delete(c.offlineState, id)
	for key := range c.chanStates {
		if strings.HasPrefix(key, id+":") {
			delete(c.chanStates, key)
		}
	}
	c.mu.Unlock()
	log.Info().Str("id", id).Msg("Shelly device removed")
}

// SyncDevices makes the registered devices match specs: new devices are added,
// changed ones updated and missing ones removed.
func (c *Client) SyncDevices(specs []DeviceSpec) {
	want := make(map[string]DeviceSpec, len(specs))
	for _, s := range specs {
		want[s.ID] = s
	}

	c.mu.RLock()
	existing := make([]string, 0, len(c.devices))
	for id := range c.devices {
		existing = append(existing, id)
	}
	c.mu.RUnlock()

	for _, id := range existing {
		if _, ok := want[id]; !ok {
			c.RemoveDevice(id)
		}
	}
	for _, spec := range specs {
		if !c.UpdateDevice(spec) {
			c.AddDevice(spec)
		}
	}
}

// IsConfigured returns true if at least one device is configured
func (c *Client) IsConfigured() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.devices) > 0
}

// HasDevice reports whether a device with the given ID is registered.
func (c *Client) HasDevice(id string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.devices[id]
	return ok
}

func (c *Client) deviceSnapshot(id string) (DeviceInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	dev, ok := c.devices[id]
	if !ok {
		return DeviceInfo{}, false
	}
	return *dev, true
}

func (c *Client) deviceSnapshots() []DeviceInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	list := make([]DeviceInfo, 0, len(c.devices))
	for _, d := range c.devices {
		list = append(list, *d)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ConfigID < list[j].ConfigID })
	return list
}

// GetDevices returns the last known state of all devices. Devices that have
// never been polled are queried live.
func (c *Client) GetDevices() []*models.Device {
	infos := c.deviceSnapshots()
	result := make([]*models.Device, 0, len(infos))
	var missing []DeviceInfo

	c.mu.RLock()
	for _, d := range infos {
		if cached, ok := c.cache[d.ConfigID]; ok {
			result = append(result, cloneDevice(cached))
		} else {
			missing = append(missing, d)
		}
	}
	c.mu.RUnlock()

	if len(missing) > 0 {
		result = append(result, c.refreshDevices(missing)...)
	}

	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// RefreshAll queries every device live, updates the cache and returns the result.
func (c *Client) RefreshAll() []*models.Device {
	return c.refreshDevices(c.deviceSnapshots())
}

func (c *Client) refreshDevices(infos []DeviceInfo) []*models.Device {
	var wg sync.WaitGroup
	results := make([]*models.Device, len(infos))
	for i := range infos {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = c.refreshDevice(infos[i])
		}(i)
	}
	wg.Wait()
	return results
}

// refreshDevice queries one device and stores the result in the cache.
func (c *Client) refreshDevice(d DeviceInfo) *models.Device {
	device, inputs := c.queryDevice(&d)
	c.mu.Lock()
	c.cache[d.ConfigID] = cloneDevice(device)
	if device.Online {
		c.inputCache[d.ConfigID] = inputs
		c.seedBaselineLocked(device)
	}
	c.mu.Unlock()
	return device
}

// GetDevice returns the last known state of a device by config ID.
func (c *Client) GetDevice(id string) (*models.Device, error) {
	d, ok := c.deviceSnapshot(id)
	if !ok {
		return nil, fmt.Errorf("device not found: %s", id)
	}
	c.mu.RLock()
	cached, ok := c.cache[id]
	c.mu.RUnlock()
	if ok {
		return cloneDevice(cached), nil
	}
	return c.refreshDevice(d), nil
}

// GetDeviceLive queries a device, updates the cache and emits change events.
func (c *Client) GetDeviceLive(id string) (*models.Device, error) {
	d, ok := c.deviceSnapshot(id)
	if !ok {
		return nil, fmt.Errorf("device not found: %s", id)
	}
	device := c.refreshDevice(d)
	c.emitChanges(device)
	return device, nil
}

// SetDeviceState sends a command to a Shelly device
func (c *Client) SetDeviceState(id string, cmd models.DeviceCommand) error {
	d, ok := c.deviceSnapshot(id)
	if !ok {
		return fmt.Errorf("device not found: %s", id)
	}
	if err := validateCommand(cmd); err != nil {
		return err
	}

	if d.Generation >= 2 {
		return c.setDeviceStateGen2(&d, cmd)
	}
	return c.setDeviceStateGen1(&d, cmd)
}

func validateCommand(cmd models.DeviceCommand) error {
	if cmd.Brightness != nil && (*cmd.Brightness < 0 || *cmd.Brightness > 100) {
		return fmt.Errorf("brightness must be between 0 and 100")
	}
	if cmd.Position != nil && (*cmd.Position < 0 || *cmd.Position > 100) {
		return fmt.Errorf("position must be between 0 and 100")
	}
	if cmd.Channel != nil && (*cmd.Channel < 0 || *cmd.Channel > 15) {
		return fmt.Errorf("channel must be between 0 and 15")
	}
	for name, v := range map[string]*int{"red": cmd.Red, "green": cmd.Green, "blue": cmd.Blue, "white": cmd.White} {
		if v != nil && (*v < 0 || *v > 255) {
			return fmt.Errorf("%s must be between 0 and 255", name)
		}
	}
	if cmd.On == nil && cmd.Brightness == nil && cmd.Position == nil && cmd.ColorTemp == nil &&
		cmd.Red == nil && cmd.Green == nil && cmd.Blue == nil && cmd.White == nil {
		return fmt.Errorf("no actionable command")
	}
	return nil
}

// GetInputs returns the last known inputs (buttons/switches) of all devices.
func (c *Client) GetInputs() []*models.Input {
	// Ensure every device has been queried at least once.
	c.GetDevices()

	c.mu.RLock()
	defer c.mu.RUnlock()
	ids := make([]string, 0, len(c.inputCache))
	for id := range c.inputCache {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	results := make([]*models.Input, 0)
	for _, id := range ids {
		for _, in := range c.inputCache[id] {
			cp := *in
			results = append(results, &cp)
		}
	}
	return results
}

// Events returns a read-only channel of device events
func (c *Client) Events() <-chan Event {
	return c.eventChan
}

// PushEvent injects an external event (e.g. from MQTT) into the event channel.
func (c *Client) PushEvent(event Event) {
	select {
	case c.eventChan <- event:
	default:
		log.Warn().Str("id", event.DeviceID).Msg("Event channel full, dropping event")
	}
}

// Close stops the client and releases resources
func (c *Client) Close() {
	c.stopOnce.Do(func() {
		close(c.stopChan)
		c.httpClient.CloseIdleConnections()
	})
}

// --- HTTP transport ---

// doRequest performs an HTTP request against a device, handling Basic (Gen1)
// and Digest (Gen2+) authentication and non-2xx status codes.
func (c *Client) doRequest(d *DeviceInfo, method, rawURL string, body []byte) ([]byte, error) {
	build := func() (*http.Request, error) {
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequest(method, rawURL, rdr)
		if err != nil {
			return nil, err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if d.Password != "" && d.Generation < 2 {
			user := d.Username
			if user == "" {
				user = "admin"
			}
			req.SetBasicAuth(user, d.Password)
		}
		return req, nil
	}

	req, err := build()
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, rawURL, err)
	}

	if resp.StatusCode == http.StatusUnauthorized && d.Password != "" {
		challenge, ok := parseDigestChallenge(resp.Header.Get("WWW-Authenticate"))
		drain(resp)
		if ok {
			user := d.Username
			if user == "" {
				user = "admin"
			}
			req, err = build()
			if err != nil {
				return nil, err
			}
			authz, err := digestAuthorization(challenge, user, d.Password, method, req.URL.RequestURI())
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", authz)
			resp, err = c.httpClient.Do(req)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", method, rawURL, err)
			}
		} else {
			return nil, fmt.Errorf("%s %s: authentication required", method, rawURL)
		}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", rawURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("%s %s: HTTP %d %s", method, rawURL, resp.StatusCode, msg)
	}
	return data, nil
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
	resp.Body.Close()
}

func decodeJSONObject(data []byte, src string) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON from %s: %w", src, err)
	}
	return result, nil
}

// --- Gen2+ RPC helpers ---

func (c *Client) rpcGet(d *DeviceInfo, method string) (map[string]interface{}, error) {
	rawURL := fmt.Sprintf("http://%s/rpc/%s", d.IP, method)
	data, err := c.doRequest(d, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return decodeJSONObject(data, rawURL)
}

func (c *Client) rpcPost(d *DeviceInfo, method string, payload interface{}) (map[string]interface{}, error) {
	rawURL := fmt.Sprintf("http://%s/rpc", d.IP)

	rpcRequest := map[string]interface{}{
		"id":     1,
		"method": method,
		"params": payload,
	}
	body, err := json.Marshal(rpcRequest)
	if err != nil {
		return nil, fmt.Errorf("marshaling RPC request: %w", err)
	}

	data, err := c.doRequest(d, http.MethodPost, rawURL, body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	rpcResp, err := decodeJSONObject(data, rawURL)
	if err != nil {
		return nil, err
	}

	if rpcErr, ok := rpcResp["error"].(map[string]interface{}); ok {
		return nil, fmt.Errorf("RPC error for %s: %v", method, rpcErr["message"])
	}
	if result, ok := rpcResp["result"].(map[string]interface{}); ok {
		return result, nil
	}
	return rpcResp, nil
}

// --- Gen1 HTTP helpers ---

func (c *Client) gen1Get(d *DeviceInfo, path string) (map[string]interface{}, error) {
	rawURL := fmt.Sprintf("http://%s/%s", d.IP, path)
	data, err := c.doRequest(d, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return decodeJSONObject(data, rawURL)
}

// --- Querying ---

// queryDevice queries a single device and returns its state and inputs.
func (c *Client) queryDevice(d *DeviceInfo) (*models.Device, []*models.Input) {
	device := &models.Device{
		ID:         d.ConfigID,
		Name:       d.Name,
		IP:         d.IP,
		Generation: d.Generation,
		Online:     false,
	}

	var inputs []*models.Input
	if d.Generation >= 2 {
		inputs = c.queryDeviceGen2(d, device)
	} else {
		inputs = c.queryDeviceGen1(d, device)
	}
	return device, inputs
}

func (c *Client) markOffline(d *DeviceInfo, gen string, err error) {
	c.mu.Lock()
	wasOffline := c.offlineState[d.ConfigID]
	c.offlineState[d.ConfigID] = true
	c.mu.Unlock()
	if !wasOffline {
		log.Warn().Err(err).Str("id", d.ConfigID).Str("ip", d.IP).Msgf("Device unreachable (%s)", gen)
	} else {
		log.Debug().Str("id", d.ConfigID).Msgf("Device still unreachable (%s)", gen)
	}
}

func (c *Client) markOnline(d *DeviceInfo, gen string) {
	c.mu.Lock()
	wasOffline := c.offlineState[d.ConfigID]
	c.offlineState[d.ConfigID] = false
	c.mu.Unlock()
	if wasOffline {
		log.Info().Str("id", d.ConfigID).Str("ip", d.IP).Msgf("Device back online (%s)", gen)
	}
}

func (c *Client) deviceInfoGen2(d *DeviceInfo) (cachedInfo, error) {
	c.mu.RLock()
	info, ok := c.infoCache[d.ConfigID]
	c.mu.RUnlock()
	if ok && time.Since(info.fetched) < deviceInfoTTL {
		return info, nil
	}

	raw, err := c.rpcGet(d, "Shelly.GetDeviceInfo")
	if err != nil {
		return cachedInfo{}, err
	}
	info = cachedInfo{fetched: time.Now()}
	if model, ok := raw["model"].(string); ok {
		info.model = model
	}
	if fw, ok := raw["fw_id"].(string); ok {
		info.firmware = fw
	}
	if app, ok := raw["app"].(string); ok {
		info.app = app
	}
	c.mu.Lock()
	c.infoCache[d.ConfigID] = info
	c.mu.Unlock()
	return info, nil
}

func (c *Client) queryDeviceGen2(d *DeviceInfo, device *models.Device) []*models.Input {
	info, err := c.deviceInfoGen2(d)
	if err != nil {
		c.markOffline(d, "Gen2", err)
		return nil
	}
	device.Model = info.model
	device.FirmwareVer = info.firmware
	device.Type = appToDeviceType(info.app)

	status, err := c.rpcGet(d, "Shelly.GetStatus")
	if err != nil {
		c.markOffline(d, "Gen2", err)
		return nil
	}
	c.markOnline(d, "Gen2")
	device.Online = true

	parseGen2Status(status, device)
	return parseGen2Inputs(status, d.ConfigID, d.Name)
}

func (c *Client) queryDeviceGen1(d *DeviceInfo, device *models.Device) []*models.Input {
	status, err := c.gen1Get(d, "status")
	if err != nil {
		c.markOffline(d, "Gen1", err)
		return nil
	}
	c.markOnline(d, "Gen1")
	device.Online = true

	parseGen1Status(status, device)
	return parseGen1Inputs(status, d.ConfigID, d.Name)
}

// --- Commands ---

func (c *Client) setDeviceStateGen2(dev *DeviceInfo, cmd models.DeviceCommand) error {
	channelID := 0
	if cmd.Channel != nil {
		channelID = *cmd.Channel
	}

	// Colour commands (RGB / RGBW components)
	if cmd.Red != nil || cmd.Green != nil || cmd.Blue != nil || cmd.White != nil {
		payload := map[string]interface{}{"id": channelID}
		if cmd.On != nil {
			payload["on"] = *cmd.On
		}
		if cmd.Brightness != nil {
			payload["brightness"] = int(*cmd.Brightness)
		}
		if cmd.Red != nil && cmd.Green != nil && cmd.Blue != nil {
			payload["rgb"] = []int{*cmd.Red, *cmd.Green, *cmd.Blue}
		}
		method := "RGB.Set"
		if cmd.White != nil {
			payload["white"] = *cmd.White
			method = "RGBW.Set"
		}
		if _, err := c.rpcPost(dev, method, payload); err != nil {
			return fmt.Errorf("colour command failed for %s: %w", dev.ConfigID, err)
		}
		log.Info().Str("id", dev.ConfigID).Int("channel", channelID).Msg("Colour set (Gen2)")
		return nil
	}

	if cmd.Position != nil {
		payload := map[string]interface{}{
			"id":  channelID,
			"pos": *cmd.Position,
		}
		if _, err := c.rpcPost(dev, "Cover.GoToPosition", payload); err != nil {
			return fmt.Errorf("cover command failed for %s: %w", dev.ConfigID, err)
		}
		log.Info().Str("id", dev.ConfigID).Int("channel", channelID).Int("pos", *cmd.Position).Msg("Cover position set (Gen2)")
		return nil
	}

	if cmd.Brightness != nil || cmd.ColorTemp != nil {
		payload := map[string]interface{}{"id": channelID}
		if cmd.On != nil {
			payload["on"] = *cmd.On
		} else if cmd.Brightness != nil {
			payload["on"] = *cmd.Brightness > 0
		}
		if cmd.Brightness != nil {
			payload["brightness"] = int(*cmd.Brightness)
		}
		method := "Light.Set"
		if cmd.ColorTemp != nil {
			payload["ct"] = *cmd.ColorTemp
			method = "CCT.Set"
		}
		if _, err := c.rpcPost(dev, method, payload); err != nil {
			return fmt.Errorf("light command failed for %s: %w", dev.ConfigID, err)
		}
		log.Info().Str("id", dev.ConfigID).Int("channel", channelID).Interface("payload", payload).Msg("Light set (Gen2)")
		return nil
	}

	if cmd.On != nil {
		payload := map[string]interface{}{
			"id": channelID,
			"on": *cmd.On,
		}
		// Pick the component from the cached device type; fall back to trying both.
		methods := []string{"Switch.Set", "Light.Set"}
		if cached, err := c.GetDevice(dev.ConfigID); err == nil {
			switch cached.Type {
			case "light", "dimmer":
				methods = []string{"Light.Set", "Switch.Set"}
			case "rgb":
				methods = []string{"RGB.Set", "Switch.Set"}
			case "rgbw":
				methods = []string{"RGBW.Set", "Switch.Set"}
			}
		}
		var lastErr error
		for _, m := range methods {
			if _, err := c.rpcPost(dev, m, payload); err == nil {
				log.Info().Str("id", dev.ConfigID).Int("channel", channelID).Bool("on", *cmd.On).Str("method", m).Msg("Output set (Gen2)")
				return nil
			} else {
				lastErr = err
			}
		}
		return fmt.Errorf("switch/light command failed for %s: %w", dev.ConfigID, lastErr)
	}

	return fmt.Errorf("no actionable command for device %s", dev.ConfigID)
}

func (c *Client) setDeviceStateGen1(dev *DeviceInfo, cmd models.DeviceCommand) error {
	channelID := 0
	if cmd.Channel != nil {
		channelID = *cmd.Channel
	}

	if cmd.Red != nil || cmd.Green != nil || cmd.Blue != nil || cmd.White != nil {
		var sb strings.Builder
		fmt.Fprintf(&sb, "color/%d?", channelID)
		if cmd.On != nil {
			fmt.Fprintf(&sb, "turn=%s&", onOff(*cmd.On))
		}
		if cmd.Red != nil {
			fmt.Fprintf(&sb, "red=%d&", *cmd.Red)
		}
		if cmd.Green != nil {
			fmt.Fprintf(&sb, "green=%d&", *cmd.Green)
		}
		if cmd.Blue != nil {
			fmt.Fprintf(&sb, "blue=%d&", *cmd.Blue)
		}
		if cmd.White != nil {
			fmt.Fprintf(&sb, "white=%d&", *cmd.White)
		}
		if cmd.Brightness != nil {
			fmt.Fprintf(&sb, "gain=%d&", int(*cmd.Brightness))
		}
		if _, err := c.gen1Get(dev, strings.TrimRight(sb.String(), "&?")); err != nil {
			return fmt.Errorf("colour command failed for %s: %w", dev.ConfigID, err)
		}
		log.Info().Str("id", dev.ConfigID).Int("channel", channelID).Msg("Colour set (Gen1)")
		return nil
	}

	if cmd.Position != nil {
		path := fmt.Sprintf("roller/%d?go=to_pos&roller_pos=%d", channelID, *cmd.Position)
		if _, err := c.gen1Get(dev, path); err != nil {
			return fmt.Errorf("roller command failed for %s: %w", dev.ConfigID, err)
		}
		log.Info().Str("id", dev.ConfigID).Int("channel", channelID).Int("pos", *cmd.Position).Msg("Roller position set (Gen1)")
		return nil
	}

	if cmd.Brightness != nil {
		on := true
		if cmd.On != nil {
			on = *cmd.On
		} else {
			on = *cmd.Brightness > 0
		}
		path := fmt.Sprintf("light/%d?turn=%s&brightness=%d", channelID, onOff(on), int(*cmd.Brightness))
		if _, err := c.gen1Get(dev, path); err != nil {
			return fmt.Errorf("dimmer command failed for %s: %w", dev.ConfigID, err)
		}
		log.Info().Str("id", dev.ConfigID).Int("channel", channelID).Bool("on", on).Float64("bri", *cmd.Brightness).Msg("Dimmer set (Gen1)")
		return nil
	}

	if cmd.On != nil {
		component := "relay"
		if cached, err := c.GetDevice(dev.ConfigID); err == nil {
			switch cached.Type {
			case "light", "dimmer":
				component = "light"
			case "rgb", "rgbw":
				component = "color"
			}
		}
		path := fmt.Sprintf("%s/%d?turn=%s", component, channelID, onOff(*cmd.On))
		if _, err := c.gen1Get(dev, path); err != nil {
			return fmt.Errorf("%s command failed for %s: %w", component, dev.ConfigID, err)
		}
		log.Info().Str("id", dev.ConfigID).Int("channel", channelID).Bool("on", *cmd.On).Msg("Output set (Gen1)")
		return nil
	}

	return fmt.Errorf("no actionable command for device %s", dev.ConfigID)
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// cloneDevice returns a deep copy of a device.
func cloneDevice(d *models.Device) *models.Device {
	if d == nil {
		return nil
	}
	cp := *d
	cp.Channels = append([]models.Channel(nil), d.Channels...)
	cp.State = cloneState(d.State)
	return &cp
}

func cloneState(s models.DeviceState) models.DeviceState {
	cp := s
	cp.Temperature = cloneFloat(s.Temperature)
	cp.Humidity = cloneFloat(s.Humidity)
	cp.Illuminance = cloneFloat(s.Illuminance)
	cp.Battery = cloneFloat(s.Battery)
	cp.Smoke = cloneBoolPtr(s.Smoke)
	cp.Flood = cloneBoolPtr(s.Flood)
	return cp
}

func cloneFloat(f *float64) *float64 {
	if f == nil {
		return nil
	}
	v := *f
	return &v
}

func cloneBoolPtr(b *bool) *bool {
	if b == nil {
		return nil
	}
	v := *b
	return &v
}
