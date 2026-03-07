package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/models"
)

// Webhook empfängt HTTP-Meldungen von Shelly-Geräten (Action-URLs).
//
// Shelly-Geräte können so konfiguriert werden, dass sie bei Ereignissen
// (z.B. Button-Druck) einen HTTP GET-Request an diesen Endpoint senden.
//
// URL-Format:
//
//	GET /api/webhook?device_id=<id>&event=<event>&channel=<n>&value=<v>
//
// Parameter:
//   - device_id (Pflicht): Shelly-Geräte-ID wie in der Konfiguration
//   - event (Pflicht): Ereignistyp, z.B. "single_push", "double_push", "long_push",
//     "btn_down", "btn_up", "toggle", "on", "off"
//   - channel (optional): Kanal-Nummer bei Mehrkanalgeräten (Standard: 0)
//   - value (optional): Zusätzlicher Wert (z.B. Helligkeit, Position)
//
// Beispiel-Action-URL in Shelly-Konfiguration:
//
//	http://<gateway>:8080/api/webhook?device_id=shelly1&event=single_push&channel=0
func (h *Handlers) Webhook(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	deviceID := q.Get("device_id")
	if deviceID == "" {
		jsonError(w, http.StatusBadRequest, "device_id ist erforderlich")
		return
	}

	event := q.Get("event")
	if event == "" {
		jsonError(w, http.StatusBadRequest, "event ist erforderlich")
		return
	}

	channel := 0
	if ch := q.Get("channel"); ch != "" {
		if v, err := strconv.Atoi(ch); err == nil {
			channel = v
		}
	}

	value := q.Get("value")

	log.Info().
		Str("device_id", deviceID).
		Str("event", event).
		Int("channel", channel).
		Str("value", value).
		Str("remote", r.RemoteAddr).
		Msg("Webhook empfangen")

	// Mapping für dieses Gerät suchen
	mapping := h.mappingManager.GetByShellyID(deviceID)
	if mapping == nil {
		log.Warn().Str("device_id", deviceID).Msg("Kein Mapping für Webhook-Gerät gefunden")
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"status":  "ok",
			"warning": "kein Mapping gefunden",
		})
		return
	}

	// Feedback-Properties erstellen
	type feedbackProp struct {
		name  string
		value interface{}
	}

	var props []feedbackProp

	// Event-Typ weiterleiten
	props = append(props, feedbackProp{"event", event})

	// Event-spezifische Werte
	switch event {
	case "on":
		props = append(props, feedbackProp{"on", 1})
	case "off":
		props = append(props, feedbackProp{"on", 0})
	case "toggle":
		props = append(props, feedbackProp{"toggle", 1})
	case "single_push":
		props = append(props, feedbackProp{"push", 1})
	case "double_push":
		props = append(props, feedbackProp{"push", 2})
	case "long_push":
		props = append(props, feedbackProp{"push", 3})
	case "btn_down":
		props = append(props, feedbackProp{"btn", 1})
	case "btn_up":
		props = append(props, feedbackProp{"btn", 0})
	}

	// Zusätzlichen Wert weiterleiten (falls vorhanden)
	if value != "" {
		props = append(props, feedbackProp{"value", value})
	}

	// Kanal als Suffix anfügen bei Mehrkanalgeräten
	loxoneID := mapping.LoxoneID
	if channel > 0 {
		loxoneID = fmt.Sprintf("%s_%d", mapping.LoxoneID, channel)
	}

	miniserverID := mapping.MiniserverID

	for _, prop := range props {
		// UDP-Feedback
		if mapping.ShouldFeedbackUDP() && h.udpSender.IsEnabled() {
			if miniserverID != "" {
				h.udpSender.Send(miniserverID, loxoneID, prop.name, prop.value)
			} else {
				h.udpSender.SendToAll(loxoneID, prop.name, prop.value)
			}
		}

		// HTTP-Feedback
		if mapping.ShouldFeedbackHTTP() && h.httpSender.IsEnabled() {
			if miniserverID != "" {
				h.httpSender.Send(miniserverID, loxoneID, prop.name, prop.value)
			} else {
				h.httpSender.SendToAll(loxoneID, prop.name, prop.value)
			}
		}

		// MQTT-Feedback
		if mapping.ShouldFeedbackMQTT() && h.mqttSender != nil && h.mqttSender.IsEnabled() {
			if miniserverID != "" {
				h.mqttSender.Send(miniserverID, loxoneID, prop.name, prop.value)
			} else {
				h.mqttSender.SendToAll(loxoneID, prop.name, prop.value)
			}
		}
	}

	// WebSocket-Broadcast für die Web-UI
	wsMsg := models.WebSocketMessage{
		Type: "webhook_event",
		Payload: map[string]interface{}{
			"device_id": deviceID,
			"event":     event,
			"channel":   channel,
			"value":     value,
			"loxone_id": loxoneID,
		},
	}
	if data, err := json.Marshal(wsMsg); err == nil {
		select {
		case h.wsHub.broadcast <- data:
		default:
			log.Warn().Msg("WebSocket-Broadcast-Kanal voll")
		}
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}
