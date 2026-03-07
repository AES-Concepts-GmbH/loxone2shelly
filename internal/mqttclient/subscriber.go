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

// Subscriber connects to an MQTT broker and emits shelly.Event for device state changes.
type Subscriber struct {
	client      mqtt.Client
	shellyClient *shelly.Client
	deviceMap   map[string]string // mqtt_id → config_id
	topicPrefix string
	mu          sync.RWMutex
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
	if mqttID == "" {
		mqttID = configID
	}
	s.deviceMap[mqttID] = configID
	log.Debug().Str("mqtt_id", mqttID).Str("config_id", configID).Msg("MQTT device registered")
}

// Start connects to the MQTT broker and subscribes to Shelly topics.
func (s *Subscriber) Start(cfg config.MQTTConfig) error {
	s.topicPrefix = cfg.TopicPrefix
	if s.topicPrefix == "" {
		s.topicPrefix = "shellies"
	}

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(cfg.ClientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			log.Warn().Err(err).Msg("MQTT connection lost")
		}).
		SetOnConnectHandler(func(c mqtt.Client) {
			log.Info().Msg("MQTT connected, subscribing to topics")
			s.subscribe(c)
		})

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
	}
	if cfg.Password != "" {
		opts.SetPassword(cfg.Password)
	}

	s.client = mqtt.NewClient(opts)

	token := s.client.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("MQTT connect failed: %w", err)
	}

	log.Info().Str("broker", cfg.Broker).Str("prefix", s.topicPrefix).Msg("MQTT subscriber started")
	return nil
}

// subscribe sets up the topic subscription.
func (s *Subscriber) subscribe(c mqtt.Client) {
	topic := s.topicPrefix + "/#"
	token := c.Subscribe(topic, 0, s.handleMessage)
	token.Wait()
	if err := token.Error(); err != nil {
		log.Error().Err(err).Str("topic", topic).Msg("MQTT subscribe failed")
		return
	}
	log.Info().Str("topic", topic).Msg("MQTT subscribed")
}

// Close disconnects from the MQTT broker.
func (s *Subscriber) Close() {
	if s.client != nil && s.client.IsConnected() {
		s.client.Disconnect(1000)
		log.Info().Msg("MQTT subscriber stopped")
	}
}

// handleMessage routes incoming MQTT messages to the appropriate parser.
func (s *Subscriber) handleMessage(_ mqtt.Client, msg mqtt.Message) {
	topic := msg.Topic()
	payload := msg.Payload()

	// Strip prefix: "shellies/shelly1-AABB/relay/0" → "shelly1-AABB/relay/0"
	rel := strings.TrimPrefix(topic, s.topicPrefix+"/")
	if rel == topic {
		return // topic doesn't match prefix
	}

	// First segment is the device identifier
	parts := strings.SplitN(rel, "/", 2)
	if len(parts) < 2 {
		return
	}

	mqttDeviceID := parts[0]
	subTopic := parts[1]

	// Resolve to config device ID
	s.mu.RLock()
	configID, known := s.deviceMap[mqttDeviceID]
	s.mu.RUnlock()
	if !known {
		log.Debug().Str("mqtt_id", mqttDeviceID).Str("topic", topic).Msg("MQTT message from unknown device, ignoring")
		return
	}

	// Route by sub-topic
	switch {
	// Gen2+: status/switch:N, status/light:N, status/cover:N
	case strings.HasPrefix(subTopic, "status/"):
		s.handleGen2Status(configID, subTopic, payload)

	// Gen2+: events/rpc
	case subTopic == "events/rpc":
		s.handleGen2Events(configID, payload)

	// Gen1: relay/N
	case strings.HasPrefix(subTopic, "relay/"):
		s.handleGen1Relay(configID, subTopic, payload)

	// Gen1: roller/N/pos
	case strings.HasPrefix(subTopic, "roller/"):
		s.handleGen1Roller(configID, subTopic, payload)

	// Gen1: light/N/status
	case strings.HasPrefix(subTopic, "light/") && strings.HasSuffix(subTopic, "/status"):
		s.handleGen1Light(configID, payload)

	// Gen1: input/N
	case strings.HasPrefix(subTopic, "input/"):
		s.handleGen1Input(configID, subTopic, payload)

	// Gen1: sensor/temperature, sensor/humidity
	case strings.HasPrefix(subTopic, "sensor/"):
		s.handleGen1Sensor(configID, subTopic, payload)
	}
}

// --- Gen2+ handlers ---

func (s *Subscriber) handleGen2Status(configID, subTopic string, payload []byte) {
	// subTopic: "status/switch:0", "status/light:0", "status/cover:0", etc.
	component := strings.TrimPrefix(subTopic, "status/")

	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		log.Debug().Str("id", configID).Str("component", component).Msg("MQTT: invalid JSON in Gen2 status")
		return
	}

	state := models.DeviceState{}
	var deviceType string

	switch {
	case strings.HasPrefix(component, "switch:"):
		deviceType = "switch"
		if output, ok := data["output"].(bool); ok {
			state.On = output
		}
		if power, ok := data["apower"].(float64); ok {
			state.Power = power
		}
		if voltage, ok := data["voltage"].(float64); ok {
			state.Voltage = voltage
		}
		if current, ok := data["current"].(float64); ok {
			state.Current = current
		}

	case strings.HasPrefix(component, "light:"):
		deviceType = "light"
		if output, ok := data["output"].(bool); ok {
			state.On = output
		}
		if bri, ok := data["brightness"].(float64); ok {
			state.Brightness = bri
		}
		if power, ok := data["apower"].(float64); ok {
			state.Power = power
		}

	case strings.HasPrefix(component, "cover:"):
		deviceType = "roller"
		if pos, ok := data["current_pos"].(float64); ok {
			state.Position = int(pos)
		}
		if power, ok := data["apower"].(float64); ok {
			state.Power = power
		}

	default:
		return
	}

	log.Debug().Str("id", configID).Str("type", deviceType).Str("component", component).Msg("MQTT Gen2 status update")

	s.shellyClient.PushEvent(shelly.Event{
		DeviceID: configID,
		Type:     deviceType,
		Data:     state,
	})
}

func (s *Subscriber) handleGen2Events(configID string, payload []byte) {
	// Gen2 events/rpc: {"method":"NotifyStatus","params":{"ts":1234,"switch:0":{"id":0,"output":true}}}
	var msg struct {
		Method string                            `json:"method"`
		Params map[string]json.RawMessage        `json:"params"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}

	if msg.Method != "NotifyStatus" {
		return
	}

	state := models.DeviceState{}
	var deviceType string

	for key, raw := range msg.Params {
		if key == "ts" {
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal(raw, &data); err != nil {
			continue
		}

		switch {
		case strings.HasPrefix(key, "switch:"):
			deviceType = "switch"
			if output, ok := data["output"].(bool); ok {
				state.On = output
			}
			if power, ok := data["apower"].(float64); ok {
				state.Power = power
			}
		case strings.HasPrefix(key, "light:"):
			deviceType = "light"
			if output, ok := data["output"].(bool); ok {
				state.On = output
			}
			if bri, ok := data["brightness"].(float64); ok {
				state.Brightness = bri
			}
		case strings.HasPrefix(key, "cover:"):
			deviceType = "roller"
			if pos, ok := data["current_pos"].(float64); ok {
				state.Position = int(pos)
			}
		}
	}

	if deviceType == "" {
		return
	}

	log.Debug().Str("id", configID).Str("type", deviceType).Msg("MQTT Gen2 RPC event")

	s.shellyClient.PushEvent(shelly.Event{
		DeviceID: configID,
		Type:     deviceType,
		Data:     state,
	})
}

// --- Gen1 handlers ---

func (s *Subscriber) handleGen1Relay(configID, subTopic string, payload []byte) {
	// subTopic: "relay/0" → payload: "on" or "off"
	val := strings.TrimSpace(string(payload))

	state := models.DeviceState{}
	state.On = (val == "on")

	log.Debug().Str("id", configID).Bool("on", state.On).Msg("MQTT Gen1 relay update")

	s.shellyClient.PushEvent(shelly.Event{
		DeviceID: configID,
		Type:     "switch",
		Data:     state,
	})
}

func (s *Subscriber) handleGen1Roller(configID, subTopic string, payload []byte) {
	// subTopic: "roller/0/pos" → payload: "50"
	if !strings.HasSuffix(subTopic, "/pos") {
		return
	}

	pos, err := strconv.Atoi(strings.TrimSpace(string(payload)))
	if err != nil {
		return
	}

	state := models.DeviceState{Position: pos}

	log.Debug().Str("id", configID).Int("pos", pos).Msg("MQTT Gen1 roller update")

	s.shellyClient.PushEvent(shelly.Event{
		DeviceID: configID,
		Type:     "roller",
		Data:     state,
	})
}

func (s *Subscriber) handleGen1Light(configID string, payload []byte) {
	// payload: JSON {"ison":true,"brightness":50,...}
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return
	}

	state := models.DeviceState{}
	if ison, ok := data["ison"].(bool); ok {
		state.On = ison
	}
	if bri, ok := data["brightness"].(float64); ok {
		state.Brightness = bri
	}

	log.Debug().Str("id", configID).Bool("on", state.On).Float64("bri", state.Brightness).Msg("MQTT Gen1 light update")

	s.shellyClient.PushEvent(shelly.Event{
		DeviceID: configID,
		Type:     "light",
		Data:     state,
	})
}

func (s *Subscriber) handleGen1Input(configID, subTopic string, payload []byte) {
	// subTopic: "input/0" → payload: "0" or "1"
	val := strings.TrimSpace(string(payload))

	state := models.DeviceState{}
	state.On = (val == "1")

	log.Debug().Str("id", configID).Bool("on", state.On).Msg("MQTT Gen1 input update")

	s.shellyClient.PushEvent(shelly.Event{
		DeviceID: configID,
		Type:     "input",
		Data:     state,
	})
}

func (s *Subscriber) handleGen1Sensor(configID, subTopic string, payload []byte) {
	// subTopic: "sensor/temperature" or "sensor/humidity"
	val, err := strconv.ParseFloat(strings.TrimSpace(string(payload)), 64)
	if err != nil {
		return
	}

	state := models.DeviceState{}
	deviceType := "sensor"

	switch {
	case strings.HasSuffix(subTopic, "temperature"):
		state.Temperature = &val
	case strings.HasSuffix(subTopic, "humidity"):
		state.Humidity = &val
	default:
		return
	}

	log.Debug().Str("id", configID).Str("sensor", subTopic).Float64("value", val).Msg("MQTT Gen1 sensor update")

	s.shellyClient.PushEvent(shelly.Event{
		DeviceID: configID,
		Type:     deviceType,
		Data:     state,
	})
}
