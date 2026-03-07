package shelly

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"github.com/rs/zerolog/log"
)

// StartPolling starts a goroutine that polls all configured Shelly devices
// at the given interval and sends Events on state changes.
func (c *Client) StartPolling(ctx context.Context, intervalSeconds int) {
	if intervalSeconds <= 0 {
		intervalSeconds = 10
	}
	ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)

	go func() {
		log.Info().Int("interval", intervalSeconds).Msg("Shelly polling started")
		// Do an initial poll immediately
		c.pollDevices()

		for {
			select {
			case <-ticker.C:
				c.pollDevices()
			case <-ctx.Done():
				ticker.Stop()
				log.Info().Msg("Shelly polling stopped (context cancelled)")
				return
			case <-c.stopChan:
				ticker.Stop()
				log.Info().Msg("Shelly polling stopped")
				return
			}
		}
	}()
}

// pollDevices queries the status of all configured devices and emits
// Events for any state changes since the last poll.
func (c *Client) pollDevices() {
	devices := c.GetDevices()

	for _, device := range devices {
		if !device.Online {
			continue
		}

		// Serialize current state for comparison
		currentState := stateToMap(device)

		c.mu.Lock()
		lastState, exists := c.lastStates[device.ID]
		c.lastStates[device.ID] = currentState
		c.mu.Unlock()

		if !exists {
			// First poll for this device, don't emit event
			log.Debug().Str("id", device.ID).Str("type", device.Type).Msg("Initial state captured")
			continue
		}

		// Compare states
		if !reflect.DeepEqual(lastState, currentState) {
			event := Event{
				DeviceID: device.ID,
				Type:     device.Type,
				Data:     device.State,
			}

			select {
			case c.eventChan <- event:
				log.Debug().Str("id", device.ID).Str("type", device.Type).Msg("State change event emitted")
			default:
				log.Warn().Str("id", device.ID).Msg("Event channel full, dropping event")
			}
		}
	}
}

// stateToMap converts a device state to a generic map for comparison
func stateToMap(device interface{}) interface{} {
	data, err := json.Marshal(device)
	if err != nil {
		return nil
	}
	var result interface{}
	json.Unmarshal(data, &result)
	return result
}
