package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/loxone"
	"github.com/sbeyeler/loxone2shelly/internal/models"
	"github.com/sbeyeler/loxone2shelly/internal/shelly"
)

const (
	wsWriteWait      = 10 * time.Second
	wsPongWait       = 60 * time.Second
	wsPingPeriod     = (wsPongWait * 9) / 10
	wsMaxMessageSize = 64 * 1024
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // non-browser client
		}
		if _, err := url.Parse(origin); err != nil {
			return false
		}
		return originAllowed(r, origin)
	},
}

// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	conn   *websocket.Conn
	send   chan []byte
	mu     sync.Mutex
	closed bool
}

// trySend queues a message without blocking. Returns false if the client is
// closed or its buffer is full.
func (c *WebSocketClient) trySend(msg []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- msg:
		return true
	default:
		return false
	}
}

// close marks the client closed and closes its send channel exactly once.
func (c *WebSocketClient) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.send)
	}
}

// WebSocketHub manages WebSocket connections and forwards Shelly events to
// both UI clients and the Loxone feedback dispatcher.
type WebSocketHub struct {
	clients    map[*WebSocketClient]bool
	broadcast  chan []byte
	register   chan *WebSocketClient
	unregister chan *WebSocketClient
	mu         sync.Mutex
	stopChan   chan struct{}
	stopOnce   sync.Once

	shellyClient   *shelly.Client
	mappingManager *loxone.MappingManager
	dispatcher     *loxone.Dispatcher
	cmdParser      *loxone.CommandParser
}

// NewWebSocketHub creates a new WebSocket hub
func NewWebSocketHub(shellyClient *shelly.Client, mappingManager *loxone.MappingManager, dispatcher *loxone.Dispatcher) *WebSocketHub {
	return &WebSocketHub{
		clients:        make(map[*WebSocketClient]bool),
		broadcast:      make(chan []byte, 256),
		register:       make(chan *WebSocketClient),
		unregister:     make(chan *WebSocketClient),
		stopChan:       make(chan struct{}),
		shellyClient:   shellyClient,
		mappingManager: mappingManager,
		dispatcher:     dispatcher,
		cmdParser:      loxone.NewCommandParser(),
	}
}

func (h *WebSocketHub) clientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// Run starts the WebSocket hub event loop
func (h *WebSocketHub) Run() {
	go h.forwardShellyEvents()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Info().Int("clients", h.clientCount()).Msg("WebSocket client connected")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.close()
			}
			h.mu.Unlock()
			log.Info().Int("clients", h.clientCount()).Msg("WebSocket client disconnected")

		case message := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				if !client.trySend(message) {
					client.close()
					delete(h.clients, client)
					log.Warn().Msg("WebSocket client too slow, dropped")
				}
			}
			h.mu.Unlock()

		case <-h.stopChan:
			h.mu.Lock()
			for client := range h.clients {
				client.close()
				delete(h.clients, client)
			}
			h.mu.Unlock()
			return
		}
	}
}

// Stop stops the WebSocket hub (idempotent)
func (h *WebSocketHub) Stop() {
	h.stopOnce.Do(func() { close(h.stopChan) })
}

// Broadcast queues a message for all clients without blocking.
func (h *WebSocketHub) Broadcast(msg models.WebSocketMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal WebSocket message")
		return
	}
	select {
	case h.broadcast <- data:
	default:
		log.Warn().Msg("WebSocket broadcast channel full")
	}
}

// forwardShellyEvents listens for Shelly device events, forwards them to
// WebSocket clients and hands them to the Loxone feedback dispatcher.
func (h *WebSocketHub) forwardShellyEvents() {
	for {
		select {
		case event, ok := <-h.shellyClient.Events():
			if !ok {
				return
			}

			payload := map[string]interface{}{
				"device_id": event.DeviceID,
				"type":      event.Type,
				"state":     event.State,
			}
			if event.Channel != nil {
				payload["channel"] = *event.Channel
			}
			h.Broadcast(models.WebSocketMessage{Type: "device_update", Payload: payload})

			dispatchEvent(h.dispatcher, event)

		case <-h.stopChan:
			return
		}
	}
}

// dispatchEvent sends a Shelly event to Loxone.
func dispatchEvent(d *loxone.Dispatcher, event shelly.Event) {
	if d == nil {
		return
	}
	switch {
	case len(event.Props) > 0:
		d.DispatchProps(event.DeviceID, event.Channel, event.Props)
	case event.Device != nil:
		d.DispatchDevice(event.Device, event.Channel)
	}
}

// HandleWebSocket handles WebSocket upgrade requests
func (h *WebSocketHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Warn().Err(err).Str("remote", r.RemoteAddr).Msg("WebSocket upgrade failed")
		return
	}

	client := &WebSocketClient{
		conn: conn,
		send: make(chan []byte, 256),
	}

	select {
	case h.register <- client:
	case <-h.stopChan:
		conn.Close()
		return
	}

	go h.writePump(client)
	go h.readPump(client)
}

// writePump pumps messages from the hub to a WebSocket client and keeps the
// connection alive with pings.
func (h *WebSocketHub) writePump(client *WebSocketClient) {
	ticker := time.NewTicker(wsPingPeriod)
	defer func() {
		ticker.Stop()
		client.conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.send:
			_ = client.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if !ok {
				_ = client.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := client.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			_ = client.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump pumps messages from a WebSocket client to the hub
func (h *WebSocketHub) readPump(client *WebSocketClient) {
	defer func() {
		select {
		case h.unregister <- client:
		case <-h.stopChan:
		}
		client.conn.Close()
	}()

	client.conn.SetReadLimit(wsMaxMessageSize)
	_ = client.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	client.conn.SetPongHandler(func(string) error {
		return client.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	for {
		_, message, err := client.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
				log.Debug().Err(err).Msg("WebSocket read error")
			}
			return
		}
		_ = client.conn.SetReadDeadline(time.Now().Add(wsPongWait))
		h.handleMessage(client, message)
	}
}

func (h *WebSocketHub) reply(client *WebSocketClient, msg models.WebSocketMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	client.trySend(data)
}

// handleMessage processes an incoming WebSocket message
func (h *WebSocketHub) handleMessage(client *WebSocketClient, message []byte) {
	trimmed := strings.TrimSpace(string(message))
	if strings.HasPrefix(trimmed, "{") {
		var wsMsg models.WebSocketMessage
		if err := json.Unmarshal(message, &wsMsg); err != nil {
			h.sendError(client, "invalid JSON: "+err.Error())
			return
		}
		switch wsMsg.Type {
		case "command":
			h.handleJSONCommand(client, wsMsg)
		case "ping":
			h.reply(client, models.WebSocketMessage{Type: "pong"})
		default:
			h.sendError(client, "unknown message type: "+wsMsg.Type)
		}
		return
	}

	// Text command format: SET/GET target action [value]
	cmd, err := h.cmdParser.ParseText(trimmed)
	if err != nil {
		h.sendError(client, err.Error())
		return
	}
	h.executeCommand(client, cmd)
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

// resolve maps a Loxone target to a Shelly device and channel. Unknown targets
// are treated as Shelly IDs so the UI can address devices directly.
func (h *WebSocketHub) resolve(target string) (string, *int) {
	if shellyID, _, channel, ok := h.mappingManager.ResolveTarget(target); ok {
		return shellyID, channel
	}
	return target, nil
}

// executeCommand resolves a target and executes a SET or GET command
func (h *WebSocketHub) executeCommand(client *WebSocketClient, cmd *models.LoxoneCommand) {
	switch cmd.Type {
	case "command":
		shellyID, channel := h.resolve(cmd.Target)
		deviceCmd := h.cmdParser.ToDeviceCommand(cmd, channel)
		if err := h.shellyClient.SetDeviceState(shellyID, deviceCmd); err != nil {
			h.sendError(client, "command failed: "+err.Error())
			return
		}
		h.reply(client, models.WebSocketMessage{
			Type: "command_ok",
			Payload: map[string]interface{}{
				"target": cmd.Target,
				"action": cmd.Action,
			},
		})
		// Refresh the device so UI and Loxone see the new state immediately.
		go func() {
			if _, err := h.shellyClient.GetDeviceLive(shellyID); err != nil {
				log.Debug().Err(err).Str("id", shellyID).Msg("Post-command refresh failed")
			}
		}()

	case "query":
		shellyID, _ := h.resolve(cmd.Target)
		device, err := h.shellyClient.GetDevice(shellyID)
		if err != nil {
			h.sendError(client, "device not found: "+err.Error())
			return
		}
		h.reply(client, models.WebSocketMessage{Type: "device_state", Payload: device})

	default:
		h.sendError(client, "unknown command type: "+cmd.Type)
	}
}

// sendError sends an error message to a WebSocket client
func (h *WebSocketHub) sendError(client *WebSocketClient, message string) {
	h.reply(client, models.WebSocketMessage{
		Type:    "error",
		Payload: map[string]string{"error": message},
	})
}
