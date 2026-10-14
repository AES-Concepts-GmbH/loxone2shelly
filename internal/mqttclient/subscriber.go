package mqttclient

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/config"
	"github.com/sbeyeler/loxone2shelly/internal/models"
	"github.com/sbeyeler/loxone2shelly/internal/shelly"
)

const connectTimeout = 10 * time.Second

// Subscriber connects to an MQTT broker and emits shelly.Event for device state changes.
// Connecting happens in the background and never blocks the caller.
type Subscriber struct {
	client       mqtt.Client
	shellyClient *shelly.Client
	deviceMap    map[string]string // mqtt_id → config_id
	topicPrefix  string
	mu           sync.RWMutex
}

// NewSubscriber creates a new MQTT subscriber.
func NewSubscriber(shellyClient *shelly.Client) *Subscriber {
	return &Subscriber{
		shellyClient: shellyClient,
		deviceMap:    make(map[string]string),
	}
}

// RegisterDevice maps an MQTT device ID to a config device ID.
// If mqttID is empty, the configID is used as the MQTT ID.
func (s *Subscriber) RegisterDevice(configID, mqttID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Drop any previous MQTT ID of this device.
	for k, v := range s.deviceMap {
		if v == configID {
			delete(s.deviceMap, k)
		}
	}
	if mqttID == "" {
		mqttID = configID
	}
	s.deviceMap[mqttID] = configID
	log.Debug().Str("mqtt_id", mqttID).Str("config_id", configID).Msg("MQTT device registered")
}

// UnregisterDevice removes all MQTT IDs of a config device.
func (s *Subscriber) UnregisterDevice(configID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.deviceMap {
		if v == configID {
			delete(s.deviceMap, k)
		}
	}
}

// SetDevices replaces the whole device registry.
func (s *Subscriber) SetDevices(devices []config.ShellyDeviceConfig) {
	s.mu.Lock()
	s.deviceMap = make(map[string]string, len(devices))
	for _, d := range devices {
		mqttID := d.MQTTID
		if mqttID == "" {
			mqttID = d.ID
		}
		s.deviceMap[mqttID] = d.ID
	}
	s.mu.Unlock()
}

// Start connects to the MQTT broker in the background and subscribes to Shelly
// topics once connected. It returns immediately; connection problems are logged.
func (s *Subscriber) Start(cfg config.MQTTConfig) error {
	if !cfg.Enabled {
		s.swapClient(nil, "", 250)
		return nil
	}
	if cfg.Broker == "" {
		s.swapClient(nil, "", 250)
		return fmt.Errorf("mqtt broker address is empty")
	}

	prefix := cfg.TopicPrefix
	if prefix == "" {
		prefix = "shellies"
	}

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(cfg.ClientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetConnectTimeout(connectTimeout).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			log.Warn().Err(err).Msg("MQTT connection lost")
		}).
		SetOnConnectHandler(func(c mqtt.Client) {
			log.Info().Msg("MQTT connected, subscribing to topics")
			subscribe(c, prefix, s.handleMessage)
		})

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
	}
	if cfg.Password != "" {
		opts.SetPassword(cfg.Password)
	}

	client := mqtt.NewClient(opts)
	s.swapClient(client, prefix, 250)

	token := client.Connect()
	go func() {
		token.Wait()
		if err := token.Error(); err != nil {
			log.Error().Err(err).Str("broker", cfg.Broker).Msg("MQTT connect failed")
		}
	}()

	log.Info().Str("broker", cfg.Broker).Str("prefix", prefix).Msg("MQTT subscriber starting")
	return nil
}

// swapClient installs next as the active client and disconnects the previous
// one. The disconnect happens outside the lock: paho waits for in-flight
// message handlers, and handleMessage needs the read lock, so disconnecting
// under the write lock would deadlock whenever a message arrives meanwhile.
func (s *Subscriber) swapClient(next mqtt.Client, prefix string, quiesce uint) {
	s.mu.Lock()
	old := s.client
	s.client = next
	if prefix != "" {
		s.topicPrefix = prefix
	}
	s.mu.Unlock()

	if old != nil {
		old.Disconnect(quiesce)
	}
}

// Restart applies a new MQTT configuration.
func (s *Subscriber) Restart(cfg config.MQTTConfig) error {
	return s.Start(cfg)
}

// IsConnected reports whether the broker connection is up.
func (s *Subscriber) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client != nil && s.client.IsConnectionOpen()
}

func subscribe(c mqtt.Client, prefix string, handler mqtt.MessageHandler) {
	topic := prefix + "/#"
	token := c.Subscribe(topic, 0, handler)
	if !token.WaitTimeout(connectTimeout) {
		log.Error().Str("topic", topic).Msg("MQTT subscribe timed out")
		return
	}
	if err := token.Error(); err != nil {
		log.Error().Err(err).Str("topic", topic).Msg("MQTT subscribe failed")
		return
	}
	log.Info().Str("topic", topic).Msg("MQTT subscribed")
}

// Close disconnects from the MQTT broker.
func (s *Subscriber) Close() {
	s.mu.RLock()
	hadClient := s.client != nil
	s.mu.RUnlock()

	s.swapClient(nil, "", 1000)
	if hadClient {
		log.Info().Msg("MQTT subscriber stopped")
	}
}

// handleMessage routes incoming MQTT messages to the appropriate parser.
func (s *Subscriber) handleMessage(_ mqtt.Client, msg mqtt.Message) {
	topic := msg.Topic()
	payload := msg.Payload()

	s.mu.RLock()
	prefix := s.topicPrefix
	s.mu.RUnlock()

	// Strip prefix: "shellies/shelly1-AABB/relay/0" → "shelly1-AABB/relay/0"
	rel := strings.TrimPrefix(topic, prefix+"/")
	if rel == topic {
		return // topic doesn't match prefix
	}

	parts := strings.SplitN(rel, "/", 2)
	if len(parts) < 2 {
		return
	}
	mqttDeviceID := parts[0]
	subTopic := parts[1]

	s.mu.RLock()
	configID, known := s.deviceMap[mqttDeviceID]
	s.mu.RUnlock()
	if !known {
		log.Debug().Str("mqtt_id", mqttDeviceID).Str("topic", topic).Msg("MQTT message from unknown device, ignoring")
		return
	}

	for _, ev := range ParseMessage(configID, subTopic, payload) {
		// Fold the update into the cached device state so the next poll does
		// not report the same change a second time, and drop status messages
		// that carry nothing new. Input events are always forwarded.
		if ev.Type != "input" {
			if st, ok := ev.State.(map[string]interface{}); ok {
				if changed, known := s.shellyClient.ApplyState(ev.DeviceID, ev.Channel, st); known && !changed {
					log.Debug().Str("id", configID).Str("topic", topic).Msg("MQTT status unchanged, skipping")
					continue
				}
			}
		}
		s.shellyClient.PushEvent(ev)
	}
}

// ParseMessage converts a Shelly MQTT message (sub-topic below the device ID)
// into zero or more events. It is exported for testing.
func ParseMessage(configID, subTopic string, payload []byte) []shelly.Event {
	switch {
	// Gen2+: status/switch:N, status/light:N, status/cover:N, status/input:N, ...
	case strings.HasPrefix(subTopic, "status/"):
		if ev, ok := gen2Component(configID, strings.TrimPrefix(subTopic, "status/"), payload); ok {
			return []shelly.Event{ev}
		}
		return nil

	// Gen2+: events/rpc  {"method":"NotifyStatus","params":{"ts":..,"switch:0":{...}}}
	case subTopic == "events/rpc":
		return gen2Events(configID, payload)

	// Gen1: relay/N
	case strings.HasPrefix(subTopic, "relay/"):
		return gen1Relay(configID, subTopic, payload)

	// Gen1: roller/N/pos
	case strings.HasPrefix(subTopic, "roller/"):
		return gen1Roller(configID, subTopic, payload)

	// Gen1: light/N/status
	case strings.HasPrefix(subTopic, "light/") && strings.HasSuffix(subTopic, "/status"):
		return gen1Light(configID, subTopic, payload)

	// Gen1: input/N and input_event/N
	case strings.HasPrefix(subTopic, "input/"):
		return gen1Input(configID, subTopic, payload)
	case strings.HasPrefix(subTopic, "input_event/"):
		return gen1InputEvent(configID, subTopic, payload)

	// Gen1: sensor/temperature, sensor/humidity, sensor/battery
	case strings.HasPrefix(subTopic, "sensor/"):
		return gen1Sensor(configID, subTopic, payload)
	}
	return nil
}

func channelPtr(i int) *int { return &i }

func splitComponent(component string) (string, int, bool) {
	kind, idxStr, ok := strings.Cut(component, ":")
	if !ok {
		return "", 0, false
	}
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		return "", 0, false
	}
	return kind, idx, true
}

// --- Gen2+ handlers ---

// gen2Component converts a single component status (e.g. "switch:1") into an event.
func gen2Component(configID, component string, payload []byte) (shelly.Event, bool) {
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		log.Debug().Str("id", configID).Str("component", component).Msg("MQTT: invalid JSON in Gen2 status")
		return shelly.Event{}, false
	}
	return gen2ComponentFromMap(configID, component, data)
}

func gen2ComponentFromMap(configID, component string, data map[string]interface{}) (shelly.Event, bool) {
	kind, idx, ok := splitComponent(component)
	if !ok {
		return shelly.Event{}, false
	}

	state := make(map[string]interface{})
	var props []models.Prop
	add := func(uiKey, loxKey string, v interface{}) {
		state[uiKey] = v
		props = append(props, models.Prop{Name: loxKey, Value: v})
	}
	fmt1 := func(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) }
	fmt3 := func(f float64) string { return strconv.FormatFloat(f, 'f', 3, 64) }

	var eventType string
	switch kind {
	case "switch":
		eventType = "switch"
		if output, ok := data["output"].(bool); ok {
			add("on", "on", boolToInt(output))
			state["on"] = output
		}
		if p, ok := data["apower"].(float64); ok {
			add("power", "power", fmt1(p))
			state["power"] = p
		}
		if v, ok := data["voltage"].(float64); ok {
			add("voltage", "voltage", fmt1(v))
			state["voltage"] = v
		}
		if a, ok := data["current"].(float64); ok {
			add("current", "current", fmt3(a))
			state["current"] = a
		}
		if e, ok := data["aenergy"].(map[string]interface{}); ok {
			if total, ok := e["total"].(float64); ok {
				add("energy", "energy", fmt1(total))
				state["energy"] = total
			}
		}

	case "light", "cct":
		eventType = "light"
		if output, ok := data["output"].(bool); ok {
			add("on", "on", boolToInt(output))
			state["on"] = output
		}
		if bri, ok := data["brightness"].(float64); ok {
			add("brightness", "bri", int(bri))
			state["brightness"] = bri
		}
		if ct, ok := data["ct"].(float64); ok {
			add("color_temp", "ct", int(ct))
		}
		if p, ok := data["apower"].(float64); ok {
			add("power", "power", fmt1(p))
			state["power"] = p
		}

	case "rgb", "rgbw":
		eventType = kind
		if output, ok := data["output"].(bool); ok {
			add("on", "on", boolToInt(output))
			state["on"] = output
		}
		if bri, ok := data["brightness"].(float64); ok {
			add("brightness", "bri", int(bri))
			state["brightness"] = bri
		}
		if rgb, ok := data["rgb"].([]interface{}); ok && len(rgb) == 3 {
			names := []string{"red", "green", "blue"}
			for i, n := range names {
				if f, ok := rgb[i].(float64); ok {
					add(n, n, int(f))
				}
			}
		}
		if w, ok := data["white"].(float64); ok {
			add("white", "white", int(w))
		}

	case "cover":
		eventType = "roller"
		if pos, ok := data["current_pos"].(float64); ok {
			add("position", "pos", int(pos))
		}
		if p, ok := data["apower"].(float64); ok {
			add("power", "power", fmt1(p))
			state["power"] = p
		}
		if st, ok := data["state"].(string); ok {
			state["cover_state"] = st
		}

	case "input":
		eventType = "input"
		if st, ok := data["state"].(bool); ok {
			add("input", "input", boolToInt(st))
			state["input"] = st
		}
		if ev, ok := data["event"].(string); ok && ev != "" {
			add("event", "event", ev)
			if push, ok := pushCode(ev); ok {
				props = append(props, models.Prop{Name: "push", Value: push})
			}
		}

	case "temperature":
		eventType = "sensor"
		if t, ok := data["tC"].(float64); ok {
			add("temperature", "temp", fmt1(t))
			state["temperature"] = t
		}
	case "humidity":
		eventType = "sensor"
		if rh, ok := data["rh"].(float64); ok {
			add("humidity", "hum", fmt1(rh))
			state["humidity"] = rh
		}
	case "illuminance":
		eventType = "sensor"
		if lux, ok := data["lux"].(float64); ok {
			add("illuminance", "lux", strconv.FormatFloat(lux, 'f', 0, 64))
			state["illuminance"] = lux
		}
	case "smoke":
		eventType = "sensor"
		if alarm, ok := data["alarm"].(bool); ok {
			add("smoke", "smoke", boolToInt(alarm))
			state["smoke"] = alarm
		}
	case "flood":
		eventType = "sensor"
		if alarm, ok := data["alarm"].(bool); ok {
			add("flood", "flood", boolToInt(alarm))
			state["flood"] = alarm
		}
	case "devicepower":
		eventType = "sensor"
		if bat, ok := data["battery"].(map[string]interface{}); ok {
			if pct, ok := bat["percent"].(float64); ok {
				add("battery", "bat", strconv.FormatFloat(pct, 'f', 0, 64))
				state["battery"] = pct
			}
		}
	case "em":
		eventType = "meter"
		if p, ok := data["total_act_power"].(float64); ok {
			add("total_power", "total_power", fmt1(p))
			state["total_power"] = p
			state["power"] = p
		}
		for _, ph := range []string{"a", "b", "c"} {
			if p, ok := data[ph+"_act_power"].(float64); ok {
				add("power_"+ph, "power_"+ph, fmt1(p))
				state["power_"+ph] = p
			}
			if v, ok := data[ph+"_voltage"].(float64); ok {
				add("voltage_"+ph, "voltage_"+ph, fmt1(v))
				state["voltage_"+ph] = v
			}
			if a, ok := data[ph+"_current"].(float64); ok {
				add("current_"+ph, "current_"+ph, fmt3(a))
				state["current_"+ph] = a
			}
		}
	case "pm1", "em1":
		eventType = "meter"
		if p, ok := data["apower"].(float64); ok {
			add("power", "power", fmt1(p))
			state["power"] = p
		}
		if p, ok := data["act_power"].(float64); ok {
			add("power", "power", fmt1(p))
			state["power"] = p
		}
		if v, ok := data["voltage"].(float64); ok {
			add("voltage", "voltage", fmt1(v))
			state["voltage"] = v
		}
		if a, ok := data["current"].(float64); ok {
			add("current", "current", fmt3(a))
			state["current"] = a
		}
	default:
		return shelly.Event{}, false
	}

	if len(props) == 0 {
		return shelly.Event{}, false
	}

	ev := shelly.Event{
		DeviceID: configID,
		Type:     eventType,
		State:    state,
		Props:    props,
	}
	// Output components are channel-addressable; sensors are device-level.
	switch kind {
	case "switch", "light", "cct", "rgb", "rgbw", "cover", "input":
		ev.Channel = channelPtr(idx)
	}
	return ev, true
}

func gen2Events(configID string, payload []byte) []shelly.Event {
	var msg struct {
		Method string                     `json:"method"`
		Params map[string]json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil
	}

	var events []shelly.Event
	switch msg.Method {
	case "NotifyStatus", "NotifyFullStatus":
		for key, raw := range msg.Params {
			if key == "ts" {
				continue
			}
			var data map[string]interface{}
			if err := json.Unmarshal(raw, &data); err != nil {
				continue
			}
			if ev, ok := gen2ComponentFromMap(configID, key, data); ok {
				events = append(events, ev)
			}
		}
	case "NotifyEvent":
		// {"method":"NotifyEvent","params":{"ts":..,"events":[{"component":"input:0","id":0,"event":"single_push","ts":..}]}}
		rawEvents, ok := msg.Params["events"]
		if !ok {
			return nil
		}
		var list []struct {
			Component string `json:"component"`
			ID        int    `json:"id"`
			Event     string `json:"event"`
		}
		if err := json.Unmarshal(rawEvents, &list); err != nil {
			return nil
		}
		for _, e := range list {
			if !strings.HasPrefix(e.Component, "input:") || e.Event == "" {
				continue
			}
			props := []models.Prop{{Name: "event", Value: e.Event}}
			if push, ok := pushCode(e.Event); ok {
				props = append(props, models.Prop{Name: "push", Value: push})
			}
			events = append(events, shelly.Event{
				DeviceID: configID,
				Type:     "input",
				Channel:  channelPtr(e.ID),
				State:    map[string]interface{}{"event": e.Event},
				Props:    props,
			})
		}
	}
	return events
}

// pushCode maps Shelly button events to the numeric "push" property.
func pushCode(event string) (int, bool) {
	switch event {
	case "single_push", "S":
		return 1, true
	case "double_push", "SS":
		return 2, true
	case "long_push", "L":
		return 3, true
	case "triple_push", "SSS":
		return 4, true
	}
	return 0, false
}

// --- Gen1 handlers ---

func gen1Channel(subTopic string) *int {
	parts := strings.Split(subTopic, "/")
	if len(parts) < 2 {
		return nil
	}
	idx, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil
	}
	return &idx
}

func gen1Relay(configID, subTopic string, payload []byte) []shelly.Event {
	// relay/N → "on"/"off"; relay/N/power → "12.3"; relay/N/energy → Wmin
	parts := strings.Split(subTopic, "/")
	ch := gen1Channel(subTopic)
	val := strings.TrimSpace(string(payload))

	switch {
	case len(parts) == 2:
		if val != "on" && val != "off" {
			return nil
		}
		on := val == "on"
		return []shelly.Event{{
			DeviceID: configID, Type: "switch", Channel: ch,
			State: map[string]interface{}{"on": on},
			Props: []models.Prop{{Name: "on", Value: boolToInt(on)}},
		}}
	case len(parts) == 3 && parts[2] == "power":
		p, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return nil
		}
		return []shelly.Event{{
			DeviceID: configID, Type: "switch", Channel: ch,
			State: map[string]interface{}{"power": p},
			Props: []models.Prop{{Name: "power", Value: strconv.FormatFloat(p, 'f', 1, 64)}},
		}}
	case len(parts) == 3 && parts[2] == "energy":
		wmin, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return nil
		}
		wh := wmin / 60
		return []shelly.Event{{
			DeviceID: configID, Type: "switch", Channel: ch,
			State: map[string]interface{}{"energy": wh},
			Props: []models.Prop{{Name: "energy", Value: strconv.FormatFloat(wh, 'f', 1, 64)}},
		}}
	}
	return nil
}

func gen1Roller(configID, subTopic string, payload []byte) []shelly.Event {
	// roller/N/pos → "50"
	if !strings.HasSuffix(subTopic, "/pos") {
		return nil
	}
	pos, err := strconv.Atoi(strings.TrimSpace(string(payload)))
	if err != nil {
		return nil
	}
	return []shelly.Event{{
		DeviceID: configID, Type: "roller", Channel: gen1Channel(subTopic),
		State: map[string]interface{}{"position": pos},
		Props: []models.Prop{{Name: "pos", Value: pos}},
	}}
}

func gen1Light(configID, subTopic string, payload []byte) []shelly.Event {
	// light/N/status → {"ison":true,"brightness":50,...}
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil
	}
	state := make(map[string]interface{})
	var props []models.Prop
	if ison, ok := data["ison"].(bool); ok {
		state["on"] = ison
		props = append(props, models.Prop{Name: "on", Value: boolToInt(ison)})
	}
	if bri, ok := data["brightness"].(float64); ok {
		state["brightness"] = bri
		props = append(props, models.Prop{Name: "bri", Value: int(bri)})
	}
	if len(props) == 0 {
		return nil
	}
	return []shelly.Event{{
		DeviceID: configID, Type: "light", Channel: gen1Channel(subTopic),
		State: state, Props: props,
	}}
}

func gen1Input(configID, subTopic string, payload []byte) []shelly.Event {
	// input/N → "0"/"1"
	val := strings.TrimSpace(string(payload))
	if val != "0" && val != "1" {
		return nil
	}
	on := val == "1"
	return []shelly.Event{{
		DeviceID: configID, Type: "input", Channel: gen1Channel(subTopic),
		State: map[string]interface{}{"input": on},
		Props: []models.Prop{{Name: "input", Value: boolToInt(on)}},
	}}
}

func gen1InputEvent(configID, subTopic string, payload []byte) []shelly.Event {
	// input_event/N → {"event":"S","event_cnt":3}
	var data struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(payload, &data); err != nil || data.Event == "" {
		return nil
	}
	props := []models.Prop{{Name: "event", Value: data.Event}}
	if push, ok := pushCode(data.Event); ok {
		props = append(props, models.Prop{Name: "push", Value: push})
	}
	return []shelly.Event{{
		DeviceID: configID, Type: "input", Channel: gen1Channel(subTopic),
		State: map[string]interface{}{"event": data.Event},
		Props: props,
	}}
}

func gen1Sensor(configID, subTopic string, payload []byte) []shelly.Event {
	// sensor/temperature, sensor/humidity, sensor/battery
	val, err := strconv.ParseFloat(strings.TrimSpace(string(payload)), 64)
	if err != nil {
		return nil
	}
	var uiKey, loxKey string
	switch strings.TrimPrefix(subTopic, "sensor/") {
	case "temperature":
		uiKey, loxKey = "temperature", "temp"
	case "humidity":
		uiKey, loxKey = "humidity", "hum"
	case "battery":
		uiKey, loxKey = "battery", "bat"
	case "lux":
		uiKey, loxKey = "illuminance", "lux"
	default:
		return nil
	}
	return []shelly.Event{{
		DeviceID: configID, Type: "sensor",
		State: map[string]interface{}{uiKey: val},
		Props: []models.Prop{{Name: loxKey, Value: strconv.FormatFloat(val, 'f', 1, 64)}},
	}}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
