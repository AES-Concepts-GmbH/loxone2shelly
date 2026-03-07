package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/loxone"
	"github.com/sbeyeler/loxone2shelly/internal/models"
	"github.com/sbeyeler/loxone2shelly/internal/shelly"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	conn *websocket.Conn
	send chan []byte
}

// WebSocketHub manages WebSocket connections
type WebSocketHub struct {
	clients        map[*WebSocketClient]bool
	broadcast      chan []byte
	register       chan *WebSocketClient
	unregister     chan *WebSocketClient
	mu             sync.Mutex
	stopChan       chan struct{}
	shellyClient   *shelly.Client
	mappingManager *loxone.MappingManager
	udpSender      *loxone.UDPSender
	httpSender     *loxone.HTTPSender
	cmdParser      *loxone.CommandParser
}

// NewWebSocketHub creates a new WebSocket hub
func NewWebSocketHub(shellyClient *shelly.Client, mappingManager *loxone.MappingManager, udpSender *loxone.UDPSender, httpSender *loxone.HTTPSender) *WebSocketHub {
	return &WebSocketHub{
		clients:        make(map[*WebSocketClient]bool),
		broadcast:      make(chan []byte, 256),
		register:       make(chan *WebSocketClient),
		unregister:     make(chan *WebSocketClient),
		stopChan:       make(chan struct{}),
		shellyClient:   shellyClient,
		mappingManager: mappingManager,
		udpSender:      udpSender,
		httpSender:     httpSender,
		cmdParser:      loxone.NewCommandParser(),
	}
}

// Run starts the WebSocket hub event loop
func (h *WebSocketHub) Run() {
	// Start forwarding Shelly events
	go h.forwardShellyEvents()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Info().Int("clients", len(h.clients)).Msg("WebSocket client connected")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Info().Int("clients", len(h.clients)).Msg("WebSocket client disconnected")

		case message := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()

		case <-h.stopChan:
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			return
		}
	}
}

// Stop stops the WebSocket hub
func (h *WebSocketHub) Stop() {
	close(h.stopChan)
}

// forwardShellyEvents listens for Shelly device events and forwards them to WebSocket clients.
// It also sends feedback to Loxone via UDP/HTTP.
func (h *WebSocketHub) forwardShellyEvents() {
	for {
		select {
		case event, ok := <-h.shellyClient.Events():
			if !ok {
				return
			}

			// Send status update to WebSocket clients
			msg := models.WebSocketMessage{
				Type: "device_update",
				Payload: map[string]interface{}{
					"device_id": event.DeviceID,
					"type":      event.Type,
					"state":     event.Data,
				},
			}
			data, err := json.Marshal(msg)
			if err != nil {
				log.Error().Err(err).Msg("Failed to marshal WebSocket message")
				continue
			}

			select {
			case h.broadcast <- data:
			default:
				log.Warn().Msg("WebSocket broadcast channel full")
			}

			// Send feedback to Loxone
			h.sendFeedback(event)

		case <-h.stopChan:
			return
		}
	}
}

// sendFeedback sends Shelly device state changes to Loxone via UDP/HTTP
func (h *WebSocketHub) sendFeedback(event shelly.Event) {
	// Find mapping for this device
	mapping := h.mappingManager.GetByShellyID(event.DeviceID)
	if mapping == nil {
		return
	}

	// Extract state properties
	state, ok := event.Data.(models.DeviceState)
	if !ok {
		log.Warn().Str("device", event.DeviceID).Msg("Cannot extract device state for feedback")
		return
	}

	// Build feedback properties
	type feedbackProp struct {
		name  string
		value interface{}
	}

	var props []feedbackProp

	// on (0/1)
	onVal := 0
	if state.On {
		onVal = 1
	}
	props = append(props, feedbackProp{"on", onVal})

	// bri (0-100)
	if state.Brightness > 0 {
		props = append(props, feedbackProp{"bri", int(state.Brightness)})
	}

	// pos (0-100)
	if event.Type == "roller" || event.Type == "cover" {
		props = append(props, feedbackProp{"pos", state.Position})
	}

	// power (Watts)
	if state.Power > 0 {
		props = append(props, feedbackProp{"power", fmt.Sprintf("%.1f", state.Power)})
	}

	miniserverID := mapping.MiniserverID

	for _, prop := range props {
		// UDP feedback
		if mapping.ShouldFeedbackUDP() && h.udpSender.IsEnabled() {
			if miniserverID != "" {
				h.udpSender.Send(miniserverID, mapping.LoxoneID, prop.name, prop.value)
			} else {
				h.udpSender.SendToAll(mapping.LoxoneID, prop.name, prop.value)
			}
		}

		// HTTP feedback
		if mapping.ShouldFeedbackHTTP() && h.httpSender.IsEnabled() {
			if miniserverID != "" {
				h.httpSender.Send(miniserverID, mapping.LoxoneID, prop.name, prop.value)
			} else {
				h.httpSender.SendToAll(mapping.LoxoneID, prop.name, prop.value)
			}
		}
	}
}

// HandleWebSocket handles WebSocket upgrade requests
func (h *WebSocketHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("WebSocket upgrade failed")
		return
	}

	client := &WebSocketClient{
		conn: conn,
		send: make(chan []byte, 256),
	}

	h.register <- client

	go h.writePump(client)
	go h.readPump(client)
}

// writePump pumps messages from the hub to a WebSocket client
func (h *WebSocketHub) writePump(client *WebSocketClient) {
	defer client.conn.Close()

	for {
		message, ok := <-client.send
		if !ok {
			client.conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}

		if err := client.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			return
		}
	}
}

// readPump pumps messages from a WebSocket client to the hub
func (h *WebSocketHub) readPump(client *WebSocketClient) {
	defer func() {
		h.unregister <- client
		client.conn.Close()
	}()

	for {
		_, message, err := client.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Error().Err(err).Msg("WebSocket read error")
			}
			break
		}

		h.handleMessage(client, message)
	}
}

// handleMessage processes an incoming WebSocket message
func (h *WebSocketHub) handleMessage(client *WebSocketClient, message []byte) {
	// Try JSON format first
	var wsMsg models.WebSocketMessage
	if err := json.Unmarshal(message, &wsMsg); err == nil {
		switch wsMsg.Type {
		case "command":
			h.handleJSONCommand(client, wsMsg)
		case "ping":
			resp := models.WebSocketMessage{Type: "pong", Payload: nil}
			data, _ := json.Marshal(resp)
			client.send <- data
		default:
			h.sendError(client, "unknown message type: "+wsMsg.Type)
		}
		return
	}

	// Try text command format: SET/GET target action [value]
	h.handleTextCommand(client, string(message))
}

// handleJSONCommand handles a JSON command message
func (h *WebSocketHub) handleJSONCommand(client *WebSocketClient, wsMsg models.WebSocketMessage) {
	payloadBytes, err := json.Marshal(wsMsg.Payload)
	if err != nil {
		h.sendError(client, "invalid command payload")
		return
	}

	cmd, err := h.cmdParser.ParseJSON(payloadBytes)
	if err != nil {
		h.sendError(client, "invalid command: "+err.Error())
		return
	}

	h.executeCommand(client, cmd)
}

// handleTextCommand handles a text-format command (SET/GET)
func (h *WebSocketHub) handleTextCommand(client *WebSocketClient, text string) {
	cmd, err := h.cmdParser.ParseText(text)
	if err != nil {
		h.sendError(client, err.Error())
		return
	}

	h.executeCommand(client, cmd)
}

// executeCommand resolves a target and executes a SET or GET command
func (h *WebSocketHub) executeCommand(client *WebSocketClient, cmd *models.LoxoneCommand) {
	switch cmd.Type {
	case "command":
		shellyID, _, ok := h.mappingManager.ResolveTarget(cmd.Target)
		if !ok {
			// Try using target directly as Shelly ID
			shellyID = cmd.Target
		}

		deviceCmd := h.cmdParser.ToDeviceCommand(cmd)
		if err := h.shellyClient.SetDeviceState(shellyID, deviceCmd); err != nil {
			h.sendError(client, "command failed: "+err.Error())
			return
		}

		resp := models.WebSocketMessage{
			Type: "command_ok",
			Payload: map[string]interface{}{
				"target": cmd.Target,
				"action": cmd.Action,
			},
		}
		data, _ := json.Marshal(resp)
		client.send <- data

	case "query":
		shellyID, _, ok := h.mappingManager.ResolveTarget(cmd.Target)
		if !ok {
			shellyID = cmd.Target
		}

		device, err := h.shellyClient.GetDevice(shellyID)
		if err != nil {
			h.sendError(client, "device not found: "+err.Error())
			return
		}

		resp := models.WebSocketMessage{
			Type:    "device_state",
			Payload: device,
		}
		data, _ := json.Marshal(resp)
		client.send <- data
	}
}

// sendError sends an error message to a WebSocket client
func (h *WebSocketHub) sendError(client *WebSocketClient, message string) {
	resp := models.WebSocketMessage{
		Type:    "error",
		Payload: map[string]string{"error": message},
	}
	data, _ := json.Marshal(resp)
	select {
	case client.send <- data:
	default:
	}
}
