package loxone

import (
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/config"
)

const (
	mqttConnectTimeout = 10 * time.Second
	mqttPublishTimeout = 5 * time.Second
	mqttQueueSize      = 1024
)

// mqttTarget represents a Miniserver receiving feedback via MQTT
type mqttTarget struct {
	topicPrefix string
	sendAll     bool
	name        string
}

// MQTTSender publishes status updates to an MQTT broker for Loxone Miniservers to subscribe to.
// Connecting happens in the background; publishing is skipped while disconnected.
type MQTTSender struct {
	client  mqtt.Client
	targets map[string]*mqttTarget // key = MiniserverConfig.ID
	mu      sync.Mutex
	// queue serialises publishes so a slow broker never blocks the event loop.
	queue *worker
}

// NewMQTTSender creates a new MQTT sender (initially disconnected).
func NewMQTTSender() *MQTTSender {
	return &MQTTSender{
		targets: make(map[string]*mqttTarget),
		queue:   newWorker("mqtt", mqttQueueSize),
	}
}

// Configure sets up the MQTT connection and registers miniserver targets.
// It never blocks on the broker: the connection is established in the background
// with automatic retries.
func (s *MQTTSender) Configure(mqttCfg config.MQTTConfig, miniservers []config.MiniserverConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		s.client.Disconnect(250)
		s.client = nil
	}
	s.targets = make(map[string]*mqttTarget)

	if !mqttCfg.Enabled {
		log.Debug().Msg("MQTT disabled, feedback publisher not started")
		return
	}

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

	opts := mqtt.NewClientOptions().
		AddBroker(mqttCfg.Broker).
		SetClientID(mqttCfg.ClientID + "-pub").
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetConnectTimeout(mqttConnectTimeout).
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

	client := mqtt.NewClient(opts)
	s.client = client

	// With ConnectRetry the token only completes once the broker is reachable.
	// Report the outcome asynchronously instead of blocking the caller.
	token := client.Connect()
	go func() {
		token.Wait()
		if err := token.Error(); err != nil {
			log.Error().Err(err).Str("broker", mqttCfg.Broker).Msg("MQTT publisher connect failed")
		}
	}()
}

// IsEnabled returns whether at least one MQTT target is active and the broker is connected.
func (s *MQTTSender) IsEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client != nil && s.client.IsConnectionOpen() && len(s.targets) > 0
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

// Send queues a property update for a specific Miniserver.
// Topic: <prefix>/<loxone_id>/<property>  Payload: <value>
func (s *MQTTSender) Send(miniserverID, loxoneID, property string, value interface{}) {
	s.mu.Lock()
	t, ok := s.targets[miniserverID]
	client := s.client
	s.mu.Unlock()

	if !ok {
		return
	}

	s.queue.submit(func() { publish(client, t, loxoneID, property, value) })
}

// SendToAll queues a property update for all Miniservers with send_all enabled.
func (s *MQTTSender) SendToAll(loxoneID, property string, value interface{}) {
	s.mu.Lock()
	targets := make([]*mqttTarget, 0)
	for _, t := range s.targets {
		if t.sendAll {
			targets = append(targets, t)
		}
	}
	client := s.client
	s.mu.Unlock()

	for _, t := range targets {
		s.queue.submit(func() { publish(client, t, loxoneID, property, value) })
	}
}

// publish sends a single MQTT message.
func publish(client mqtt.Client, t *mqttTarget, loxoneID, property string, value interface{}) {
	if client == nil || !client.IsConnectionOpen() {
		return
	}

	topic := fmt.Sprintf("%s/%s/%s", t.topicPrefix, loxoneID, property)
	payload := fmt.Sprintf("%v", value)

	token := client.Publish(topic, 0, false, payload)
	if !token.WaitTimeout(mqttPublishTimeout) {
		log.Warn().Str("miniserver", t.name).Str("topic", topic).Msg("MQTT publish timed out")
		return
	}
	if err := token.Error(); err != nil {
		log.Warn().Err(err).Str("miniserver", t.name).Str("topic", topic).Msg("Failed to publish MQTT feedback")
		return
	}

	log.Debug().Str("miniserver", t.name).Str("topic", topic).Str("payload", payload).Msg("MQTT feedback published")
}

// Close disconnects the MQTT client and stops the publish queue.
func (s *MQTTSender) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.queue.close()
	if s.client != nil {
		s.client.Disconnect(1000)
		s.client = nil
		log.Info().Msg("MQTT publisher stopped")
	}
}
