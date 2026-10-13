package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/config"
	"github.com/sbeyeler/loxone2shelly/internal/loxone"
	"github.com/sbeyeler/loxone2shelly/internal/models"
	"github.com/sbeyeler/loxone2shelly/internal/shelly"
)

// knownWebhookEvents lists the event names Shelly action URLs may report.
var knownWebhookEvents = map[string]bool{
	"single_push": true, "double_push": true, "triple_push": true, "long_push": true,
	"btn_down": true, "btn_up": true, "toggle": true, "on": true, "off": true,
	"open": true, "close": true, "stop": true,
}

// Webhook receives HTTP notifications from Shelly devices (action URLs).
//
// URL format:
//
//	GET /api/webhook?device_id=<id>&event=<event>&channel=<n>&value=<v>[&token=<webhook_token>]
//
// Parameters:
//   - device_id (required): Shelly device ID as configured
//   - event (required): e.g. "single_push", "double_push", "long_push", "btn_down", "btn_up", "toggle", "on", "off"
//   - channel (optional): channel index for multi-channel devices (default 0)
//   - value (optional): additional value (e.g. brightness, position)
//   - token (required when server.webhook_token is configured)
func (h *Handlers) Webhook(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	if expected := config.GetServer().WebhookToken; expected != "" {
		if !tokenMatches(q.Get("token"), expected) {
			jsonError(w, http.StatusUnauthorized, "invalid webhook token")
			return
		}
	}

	deviceID := q.Get("device_id")
	if deviceID == "" {
		jsonError(w, http.StatusBadRequest, "device_id is required")
		return
	}
	if !h.deps.Shelly.HasDevice(deviceID) {
		jsonError(w, http.StatusNotFound, "unknown device_id")
		return
	}

	event := q.Get("event")
	if event == "" {
		jsonError(w, http.StatusBadRequest, "event is required")
		return
	}
	if len(event) > 32 || !knownWebhookEvents[event] {
		jsonError(w, http.StatusBadRequest, "unknown event")
		return
	}

	channel := 0
	if ch := q.Get("channel"); ch != "" {
		v, err := strconv.Atoi(ch)
		if err != nil || v < 0 || v > 15 {
			jsonError(w, http.StatusBadRequest, "channel must be between 0 and 15")
			return
		}
		channel = v
	}

	value := q.Get("value")
	if len(value) > 64 {
		jsonError(w, http.StatusBadRequest, "value too long")
		return
	}

	log.Info().
		Str("device_id", deviceID).
		Str("event", event).
		Int("channel", channel).
		Str("value", value).
		Str("remote", r.RemoteAddr).
		Msg("Webhook received")

	props := loxone.WebhookProps(event, value)
	mappings := h.deps.Mappings.GetByShellyID(deviceID)

	sent := 0
	var loxoneID string
	for i := range mappings {
		m := &mappings[i]
		target := ""
		switch {
		case m.Channel != nil && *m.Channel == channel:
			target = m.LoxoneID
		case m.Channel == nil && channel == 0:
			target = m.LoxoneID
		case m.Channel == nil:
			// Device-level mapping on a multi-channel device: suffix the channel.
			target = fmt.Sprintf("%s_%d", m.LoxoneID, channel)
		default:
			continue
		}
		if loxoneID == "" {
			loxoneID = target
		}
		h.deps.Dispatcher.SendToMapping(m, target, props)
		sent++
	}

	// Mirror the event as an input event so cached inputs and UI stay in sync.
	ch := channel
	h.deps.Shelly.PushEvent(eventFromWebhook(deviceID, &ch, event, value))

	h.wsHub.Broadcast(models.WebSocketMessage{
		Type: "webhook_event",
		Payload: map[string]interface{}{
			"device_id": deviceID,
			"event":     event,
			"channel":   channel,
			"value":     value,
			"loxone_id": loxoneID,
		},
	})

	if sent == 0 {
		log.Warn().Str("device_id", deviceID).Msg("No mapping found for webhook device")
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"status":  "ok",
			"warning": "no mapping found",
		})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{"status": "ok", "mappings": sent})
}

// eventFromWebhook converts a webhook call into an input event for UI clients.
// Loxone feedback is handled separately by the webhook handler, so Props stay empty.
func eventFromWebhook(deviceID string, channel *int, event, value string) shelly.Event {
	state := map[string]interface{}{"event": event}
	if value != "" {
		state["value"] = value
	}
	switch event {
	case "btn_down":
		state["input"] = true
	case "btn_up":
		state["input"] = false
	}
	return shelly.Event{
		DeviceID: deviceID,
		Type:     "input",
		Channel:  channel,
		State:    state,
	}
}
