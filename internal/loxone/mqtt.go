package loxone

import (
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/config"
)

// mqttTarget represents a Miniserver receiving feedback via MQTT
type mqttTarget struct {
	topicPrefix string
	sendAll     bool
	name        string
}

// MQTTSender publishes status updates to an MQTT broker for Loxone Miniservers to subscribe to.
type MQTTSender struct {
	client  mqtt.Client
	targets map[string]*mqttTarget // key = MiniserverConfig.ID
	mu      sync.Mutex
}

// NewMQTTSender creates a new MQTT sender (initially disconnected).
func NewMQTTSender() *MQTTSender {
	return &MQTTSender{
		targets: make(map[string]*mqttTarget),
	}
}

// Configure sets up the MQTT connection and registers miniserver targets.
// It uses the global MQTT broker config and per-miniserver MQTT settings.
func (s *MQTTSender) Configure(mqttCfg config.MQTTConfig, miniservers []config.MiniserverConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Disconnect previous client if any
	if s.client != nil && s.client.IsConnected() {
		s.client.Disconnect(500)
	}

	s.targets = make(map[string]*mqttTarget)

	// Check if any miniserver has MQTT feedback enabled
	hasAny := false
	for _, ms := range miniservers {
		if ms.MQTTEnabled {
			hasAny = true
			break
		}
	}
	if !hasAny {
		log.Debug().Msg("No miniservers with MQTT feedback enabled")
		return
	}

	// Connect to MQTT broker
	clientID := mqttCfg.ClientID + "-pub"
	opts := mqtt.NewClientOptions().
		AddBroker(mqttCfg.Broker).
		SetClientID(clientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			log.Warn().Err(err).Msg("MQTT publisher connection lost")
		}).
		SetOnConnectHandler(func(_ mqtt.Client) {
			log.Info().Msg("MQTT publisher connected")
		})

	if mqttCfg.Username != "" {
		opts.SetUsername(mqttCfg.Username)
	}
	if mqttCfg.Password != "" {
		opts.SetPassword(mqttCfg.Password)
	}

	s.client = mqtt.NewClient(opts)
	token := s.client.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		log.Error().Err(err).Msg("MQTT publisher connect failed")
		return
	}

	// Register targets
	for _, ms := range miniservers {
		if !ms.MQTTEnabled {
			continue
		}

		prefix := ms.MQTTTopicPrefix
		if prefix == "" {
			prefix = "loxone2shelly"
		}

		s.targets[ms.ID] = &mqttTarget{
			topicPrefix: prefix,
			sendAll:     ms.SendAll,
			name:        ms.Name,
		}

		log.Info().Str("id", ms.ID).Str("name", ms.Name).Str("prefix", prefix).Msg("MQTT feedback configured for miniserver")
	}
}

// IsEnabled returns whether at least one MQTT target is active.
func (s *MQTTSender) IsEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client != nil && s.client.IsConnected() && len(s.targets) > 0
}

// HasSendAll returns true if at least one target has send_all enabled.
func (s *MQTTSender) HasSendAll() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.targets {
		if t.sendAll {
			return true
		}
	}
	return false
}

// Send publishes a property update for a specific Miniserver.
// Topic: <prefix>/<loxone_id>/<property>  Payload: <value>
func (s *MQTTSender) Send(miniserverID, loxoneID, property string, value interface{}) {
	s.mu.Lock()
	t, ok := s.targets[miniserverID]
	s.mu.Unlock()

	if !ok {
		return
	}

	s.publish(t, loxoneID, property, value)
}

// SendToAll publishes a property update to all Miniservers with send_all enabled.
func (s *MQTTSender) SendToAll(loxoneID, property string, value interface{}) {
	s.mu.Lock()
	targets := make([]*mqttTarget, 0)
	for _, t := range s.targets {
		if t.sendAll {
			targets = append(targets, t)
		}
	}
	s.mu.Unlock()

	for _, t := range targets {
		s.publish(t, loxoneID, property, value)
	}
}

// publish sends a single MQTT message.
func (s *MQTTSender) publish(t *mqttTarget, loxoneID, property string, value interface{}) {
	if s.client == nil || !s.client.IsConnected() {
		return
	}

	topic := fmt.Sprintf("%s/%s/%s", t.topicPrefix, loxoneID, property)
	payload := fmt.Sprintf("%v", value)

	token := s.client.Publish(topic, 0, false, payload)
	token.Wait()
	if err := token.Error(); err != nil {
		log.Warn().Err(err).Str("miniserver", t.name).Str("topic", topic).Msg("Failed to publish MQTT feedback")
		return
	}

	log.Debug().Str("miniserver", t.name).Str("topic", topic).Str("payload", payload).Msg("MQTT feedback published")
}

// Close disconnects the MQTT client.
func (s *MQTTSender) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil && s.client.IsConnected() {
		s.client.Disconnect(1000)
		log.Info().Msg("MQTT publisher stopped")
	}
}
