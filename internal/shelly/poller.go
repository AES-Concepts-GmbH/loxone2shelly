package shelly

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/models"
)

// SetPollInterval changes the polling interval at runtime.
func (c *Client) SetPollInterval(intervalSeconds int) {
	if intervalSeconds <= 0 {
		intervalSeconds = 10
	}
	d := time.Duration(intervalSeconds) * time.Second
	c.mu.Lock()
	changed := c.pollInterval != d
	c.pollInterval = d
	c.mu.Unlock()
	if changed {
		select {
		case c.pollReset <- struct{}{}:
		default:
		}
		log.Info().Int("interval", intervalSeconds).Msg("Shelly poll interval changed")
	}
}

func (c *Client) currentPollInterval() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.pollInterval
}

// StartPolling polls all configured Shelly devices at the configured interval
// and emits Events on state changes. It blocks until ctx is cancelled or the
// client is closed.
func (c *Client) StartPolling(ctx context.Context, intervalSeconds int) {
	c.SetPollInterval(intervalSeconds)

	log.Info().Int("interval", int(c.currentPollInterval().Seconds())).Msg("Shelly polling started")
	c.pollDevices()

	ticker := time.NewTicker(c.currentPollInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.pollDevices()
		case <-c.pollReset:
			ticker.Reset(c.currentPollInterval())
		case <-ctx.Done():
			log.Info().Msg("Shelly polling stopped (context cancelled)")
			return
		case <-c.stopChan:
			log.Info().Msg("Shelly polling stopped")
			return
		}
	}
}

// pollDevices queries the status of all configured devices and emits
// Events for any state changes since the last poll.
func (c *Client) pollDevices() {
	for _, device := range c.RefreshAll() {
		c.emitChanges(device)
	}
}

// seedBaselineLocked records the current state as the comparison baseline for
// devices seen for the first time, so the first real change emits an event.
// The caller must hold c.mu.
func (c *Client) seedBaselineLocked(device *models.Device) {
	if _, ok := c.lastStates[device.ID]; !ok {
		c.lastStates[device.ID] = stateToMap(device)
	}
	if len(device.Channels) > 1 {
		for _, ch := range device.Channels {
			key := channelKey(device.ID, ch.Index)
			if _, ok := c.chanStates[key]; !ok {
				c.chanStates[key] = stateToMap(ch)
			}
		}
	}
}

// emitChanges compares a fresh device snapshot with the last known state and
// emits device-level and channel-level events for every change.
func (c *Client) emitChanges(device *models.Device) {
	if device == nil || !device.Online {
		return
	}

	currentState := stateToMap(device)

	c.mu.Lock()
	lastState, exists := c.lastStates[device.ID]
	c.lastStates[device.ID] = currentState

	type chanChange struct {
		index int
		first bool
	}
	var changedChannels []chanChange
	if len(device.Channels) > 1 {
		for _, ch := range device.Channels {
			key := channelKey(device.ID, ch.Index)
			cur := stateToMap(ch)
			last, had := c.chanStates[key]
			c.chanStates[key] = cur
			if !had {
				changedChannels = append(changedChannels, chanChange{ch.Index, true})
			} else if !reflect.DeepEqual(last, cur) {
				changedChannels = append(changedChannels, chanChange{ch.Index, false})
			}
		}
	}
	c.mu.Unlock()

	if !exists {
		log.Debug().Str("id", device.ID).Str("type", device.Type).Msg("Initial state captured")
		return
	}

	if !reflect.DeepEqual(lastState, currentState) {
		c.PushEvent(Event{
			DeviceID: device.ID,
			Type:     device.Type,
			State:    device.State,
			Device:   cloneDevice(device),
		})
		log.Debug().Str("id", device.ID).Str("type", device.Type).Msg("State change event emitted")
	}

	for _, cc := range changedChannels {
		if cc.first {
			continue
		}
		idx := cc.index
		var ch *models.Channel
		for i := range device.Channels {
			if device.Channels[i].Index == idx {
				ch = &device.Channels[i]
				break
			}
		}
		if ch == nil {
			continue
		}
		c.PushEvent(Event{
			DeviceID: device.ID,
			Type:     ch.Type,
			Channel:  &idx,
			State:    channelState(ch),
			Device:   cloneDevice(device),
		})
	}
}

func channelKey(id string, index int) string {
	return id + ":" + strconv.Itoa(index)
}

// channelState converts a channel into the partial state map used by UI clients.
func channelState(ch *models.Channel) map[string]interface{} {
	m := map[string]interface{}{"on": ch.On}
	switch ch.Type {
	case "light":
		m["brightness"] = ch.Brightness
	case "cover":
		m["position"] = ch.Position
	}
	m["power"] = ch.Power
	m["voltage"] = ch.Voltage
	m["current"] = ch.Current
	m["energy"] = ch.Energy
	return m
}

// stateToMap converts a value to a generic map for comparison
func stateToMap(v interface{}) interface{} {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}
