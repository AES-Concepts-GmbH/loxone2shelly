package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sbeyeler/loxone2shelly/internal/models"
	"github.com/sbeyeler/loxone2shelly/internal/shelly"
)

func wsURL(ts *httptest.Server, query string) string {
	return "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws" + query
}

func dialWS(t *testing.T, ts *httptest.Server, query string, headers http.Header) *websocket.Conn {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL(ts, query), headers)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial: %v (status %d)", err, status)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// readUntil reads messages until one with the wanted type arrives.
func readUntil(t *testing.T, conn *websocket.Conn, wantType string) models.WebSocketMessage {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(deadline)
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read while waiting for %q: %v", wantType, err)
		}
		var msg models.WebSocketMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("invalid json %q: %v", data, err)
		}
		if msg.Type == wantType {
			return msg
		}
	}
	t.Fatalf("no %q message within deadline", wantType)
	return models.WebSocketMessage{}
}

func errorText(t *testing.T, msg models.WebSocketMessage) string {
	t.Helper()
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		t.Fatalf("error payload is not an object: %v", msg.Payload)
	}
	s, _ := payload["error"].(string)
	return s
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func newWSTestServer(t *testing.T) (*httptest.Server, *Deps, *fakeShelly) {
	t.Helper()
	dev := newFakeShelly(t)
	ts, deps := newTestServer(t, `
shelly:
  devices:
    - id: dev1
      name: Fake
      ip: `+dev.hostPort()+`
      generation: 2
mappings:
  - id: m1
    loxone_id: lox1
    shelly_id: dev1
    enabled: true
  - id: m2
    loxone_id: lox_ch1
    shelly_id: dev1
    channel: 1
    enabled: true
`)
	return ts, deps, dev
}

func TestWebSocketTextCommandsAndQueries(t *testing.T) {
	ts, _, dev := newWSTestServer(t)
	conn := dialWS(t, ts, "", nil)

	if err := conn.WriteMessage(websocket.TextMessage, []byte("SET lox1 ON")); err != nil {
		t.Fatal(err)
	}
	msg := readUntil(t, conn, "command_ok")
	if payload := msg.Payload.(map[string]interface{}); payload["target"] != "lox1" {
		t.Fatalf("unexpected payload: %v", payload)
	}
	waitFor(t, "channel 0 on", func() bool { return dev.output(0) })

	// The channel mapping routes to channel 1 of the same device.
	if err := conn.WriteMessage(websocket.TextMessage, []byte("SET lox_ch1 ON")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, conn, "command_ok")
	waitFor(t, "channel 1 on", func() bool { return dev.output(1) })

	// The post-command refresh publishes the new state to UI clients.
	update := readUntil(t, conn, "device_update")
	if update.Payload.(map[string]interface{})["device_id"] != "dev1" {
		t.Fatalf("unexpected device_update: %v", update.Payload)
	}

	if err := conn.WriteMessage(websocket.TextMessage, []byte("GET lox1 STATUS")); err != nil {
		t.Fatal(err)
	}
	state := readUntil(t, conn, "device_state")
	if state.Payload.(map[string]interface{})["id"] != "dev1" {
		t.Fatalf("unexpected device_state payload: %v", state.Payload)
	}

	if err := conn.WriteMessage(websocket.TextMessage, []byte("SET lox1 FLY")); err != nil {
		t.Fatal(err)
	}
	if e := errorText(t, readUntil(t, conn, "error")); !strings.Contains(e, "unknown action") {
		t.Fatalf("unexpected error: %s", e)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("GET nope STATUS")); err != nil {
		t.Fatal(err)
	}
	if e := errorText(t, readUntil(t, conn, "error")); !strings.Contains(e, "device not found") {
		t.Fatalf("unexpected error: %s", e)
	}
}

func TestWebSocketJSONMessages(t *testing.T) {
	ts, _, dev := newWSTestServer(t)
	conn := dialWS(t, ts, "", nil)

	send := func(v interface{}) {
		t.Helper()
		data, _ := json.Marshal(v)
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			t.Fatal(err)
		}
	}

	send(map[string]interface{}{"type": "ping"})
	readUntil(t, conn, "pong")

	send(map[string]interface{}{"type": "command", "payload": map[string]interface{}{
		"target": "dev1", "action": "set", "params": map[string]interface{}{"on": true, "channel": 1},
	}})
	readUntil(t, conn, "command_ok")
	waitFor(t, "channel 1 on via JSON", func() bool { return dev.output(1) })

	send(map[string]interface{}{"type": "command", "payload": map[string]interface{}{"params": map[string]interface{}{}}})
	if e := errorText(t, readUntil(t, conn, "error")); !strings.Contains(e, "target is required") {
		t.Fatalf("expected target error, got %s", e)
	}

	send(map[string]interface{}{"type": "command", "payload": "not an object"})
	if e := errorText(t, readUntil(t, conn, "error")); !strings.Contains(e, "invalid command") {
		t.Fatalf("expected invalid command, got %s", e)
	}

	send(map[string]interface{}{"type": "bogus"})
	if e := errorText(t, readUntil(t, conn, "error")); !strings.Contains(e, "unknown message type") {
		t.Fatalf("expected unknown type error, got %s", e)
	}

	if err := conn.WriteMessage(websocket.TextMessage, []byte("{not json")); err != nil {
		t.Fatal(err)
	}
	if e := errorText(t, readUntil(t, conn, "error")); !strings.Contains(e, "invalid JSON") {
		t.Fatalf("expected json error, got %s", e)
	}

	send(map[string]interface{}{"type": "command", "payload": map[string]interface{}{"target": "nope", "params": map[string]interface{}{"on": true}}})
	if e := errorText(t, readUntil(t, conn, "error")); !strings.Contains(e, "command failed") {
		t.Fatalf("expected command failed, got %s", e)
	}

	send(map[string]interface{}{"type": "command", "payload": map[string]interface{}{"type": "weird", "target": "dev1"}})
	if e := errorText(t, readUntil(t, conn, "error")); !strings.Contains(e, "unknown command type") {
		t.Fatalf("expected unknown command type, got %s", e)
	}
}

func TestWebSocketBroadcastsDeviceAndWebhookEvents(t *testing.T) {
	ts, deps, _ := newWSTestServer(t)
	conn := dialWS(t, ts, "", nil)

	ch := 1
	deps.Shelly.PushEvent(shelly.Event{
		DeviceID: "dev1",
		Type:     "switch",
		Channel:  &ch,
		State:    map[string]interface{}{"on": true},
		Props:    []models.Prop{{Name: "on", Value: 1}},
	})
	msg := readUntil(t, conn, "device_update")
	payload := msg.Payload.(map[string]interface{})
	if payload["device_id"] != "dev1" || payload["channel"] != float64(1) || payload["type"] != "switch" {
		t.Fatalf("unexpected device_update: %v", payload)
	}

	resp, _ := do(t, "GET", ts.URL+"/api/webhook?device_id=dev1&event=single_push&channel=1", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("webhook: %d", resp.StatusCode)
	}
	hook := readUntil(t, conn, "webhook_event")
	hp := hook.Payload.(map[string]interface{})
	if hp["event"] != "single_push" || hp["loxone_id"] != "lox_ch1" || hp["channel"] != float64(1) {
		t.Fatalf("unexpected webhook_event: %v", hp)
	}
}

func TestWebSocketRequiresTokenAndChecksOrigin(t *testing.T) {
	ts, _ := newTestServer(t, "server:\n  api_token: ws-secret\n")

	if _, resp, err := websocket.DefaultDialer.Dial(wsURL(ts, ""), nil); err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got err=%v resp=%v", err, resp)
	}

	conn := dialWS(t, ts, "?token=ws-secret", nil)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`)); err != nil {
		t.Fatal(err)
	}
	readUntil(t, conn, "pong")

	h := http.Header{"Origin": []string{"http://evil.local"}}
	if _, resp, err := websocket.DefaultDialer.Dial(wsURL(ts, "?token=ws-secret"), h); err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for foreign origin, got err=%v resp=%v", err, resp)
	}

	h = http.Header{"Origin": []string{ts.URL}}
	c2 := dialWS(t, ts, "?token=ws-secret", h)
	c2.Close()
}

func TestWebSocketServerCloseDisconnectsClients(t *testing.T) {
	ts, deps := newTestServer(t, "")
	conn := dialWS(t, ts, "", nil)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`)); err != nil {
		t.Fatal(err)
	}
	readUntil(t, conn, "pong")

	// Stopping the hub closes every client; the next read fails.
	srv := deps // keep deps referenced for clarity
	_ = srv
	hub := serverHubFor(t, ts)
	hub.Stop()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected connection to be closed after hub stop")
	}
}

func TestWebSocketClientSendAfterClose(t *testing.T) {
	c := &WebSocketClient{send: make(chan []byte, 1)}
	if !c.trySend([]byte("a")) {
		t.Fatal("first send must succeed")
	}
	if c.trySend([]byte("b")) {
		t.Fatal("full buffer must not block or succeed")
	}
	c.close()
	c.close() // idempotent
	if c.trySend([]byte("c")) {
		t.Fatal("send after close must fail, not panic")
	}
}

func TestDispatchEventHandlesEmptyEvents(t *testing.T) {
	dispatchEvent(nil, shelly.Event{})
	_, deps := newTestServer(t, "")
	dispatchEvent(deps.Dispatcher, shelly.Event{DeviceID: "x"})
}

func TestWebhookTargetsSelection(t *testing.T) {
	one := 1
	mappings := []models.Mapping{
		{ID: "dev", LoxoneID: "lox", ShellyID: "d"},
		{ID: "ch1", LoxoneID: "lox_ch1", ShellyID: "d", Channel: &one},
	}
	got := webhookTargets(mappings, 1)
	if len(got) != 1 || got[0].loxoneID != "lox_ch1" {
		t.Fatalf("channel-specific mapping must win: %+v", got)
	}
	got = webhookTargets(mappings, 0)
	if len(got) != 1 || got[0].loxoneID != "lox" {
		t.Fatalf("channel 0 goes to the device-level mapping: %+v", got)
	}
	got = webhookTargets(mappings, 2)
	if len(got) != 1 || got[0].loxoneID != "lox_2" {
		t.Fatalf("unmapped channel falls back to suffixed device-level id: %+v", got)
	}
	if got := webhookTargets(nil, 0); len(got) != 0 {
		t.Fatalf("no mappings, no targets: %+v", got)
	}
}
