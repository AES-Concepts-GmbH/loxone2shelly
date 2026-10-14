package mqttclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/sbeyeler/loxone2shelly/internal/config"
	"github.com/sbeyeler/loxone2shelly/internal/shelly"
)

// blockingClient emulates paho's Disconnect, which waits for in-flight
// message handlers: a message arrives while disconnecting and its handler
// needs the subscriber's lock.
type blockingClient struct {
	mqtt.Client
	sub          *Subscriber
	disconnected chan struct{}
}

func (b *blockingClient) IsConnectionOpen() bool { return true }

func (b *blockingClient) Disconnect(uint) {
	done := make(chan struct{})
	go func() {
		b.sub.handleMessage(nil, fakeMessage{topic: "shellies/cfg/relay/0", payload: []byte("on")})
		close(done)
	}()
	<-done
	close(b.disconnected)
}

// TestRestartDoesNotDeadlockWithInflightMessage is a regression test: Restart
// and Close used to disconnect while holding the write lock, so a handler
// blocked on the read lock kept paho's Disconnect from returning.
func TestRestartDoesNotDeadlockWithInflightMessage(t *testing.T) {
	sc := shelly.NewClient()
	defer sc.Close()
	s := NewSubscriber(sc)
	s.SetDevices([]config.ShellyDeviceConfig{{ID: "cfg"}})
	s.topicPrefix = "shellies"

	for _, step := range []func(){
		func() { _ = s.Restart(config.MQTTConfig{Enabled: false}) },
		func() { s.Close() },
	} {
		bc := &blockingClient{sub: s, disconnected: make(chan struct{})}
		s.mu.Lock()
		s.client = bc
		s.mu.Unlock()
		if !s.IsConnected() {
			t.Fatal("fake client must report connected")
		}

		finished := make(chan struct{})
		go func() {
			step()
			close(finished)
		}()
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			t.Fatal("disconnect deadlocked with an in-flight message handler")
		}
		select {
		case <-bc.disconnected:
		default:
			t.Fatal("old client was not disconnected")
		}
		if s.IsConnected() {
			t.Fatal("client must be dropped")
		}
		// The in-flight message was still delivered.
		select {
		case ev := <-sc.Events():
			if ev.DeviceID != "cfg" {
				t.Fatalf("unexpected event %+v", ev)
			}
		case <-time.After(time.Second):
			t.Fatal("in-flight message was lost")
		}
	}
}

// TestStatusMessagesAreDeduplicatedAgainstCache: a status that matches the
// cached device state is not forwarded, and one that changes it updates the
// cache so the next poll stays quiet.
func TestStatusMessagesAreDeduplicatedAgainstCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/Shelly.GetDeviceInfo":
			_, _ = w.Write([]byte(`{"app":"Plus1PM"}`))
		case "/rpc/Shelly.GetStatus":
			_, _ = w.Write([]byte(`{"switch:0":{"id":0,"output":false,"apower":0}}`))
		}
	}))
	defer srv.Close()

	sc := shelly.NewClient()
	defer sc.Close()
	sc.AddDevice(shelly.DeviceSpec{ID: "plug", IP: strings.TrimPrefix(srv.URL, "http://"), Generation: 2})
	if dev, _ := sc.GetDevice("plug"); !dev.Online {
		t.Fatal("device must be cached online")
	}

	s := NewSubscriber(sc)
	s.topicPrefix = "shellies"
	s.SetDevices([]config.ShellyDeviceConfig{{ID: "plug"}})

	// Same state as cached -> dropped.
	s.handleMessage(nil, fakeMessage{topic: "shellies/plug/status/switch:0", payload: []byte(`{"id":0,"output":false,"apower":0}`)})
	select {
	case ev := <-sc.Events():
		t.Fatalf("unchanged status must not be forwarded: %+v", ev)
	default:
	}

	// A change is forwarded once and folded into the cache.
	s.handleMessage(nil, fakeMessage{topic: "shellies/plug/status/switch:0", payload: []byte(`{"id":0,"output":true,"apower":12.5}`)})
	select {
	case ev := <-sc.Events():
		if ev.DeviceID != "plug" || len(ev.Props) == 0 {
			t.Fatalf("unexpected event %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("changed status must be forwarded")
	}
	if dev, _ := sc.GetDevice("plug"); !dev.State.On || dev.State.Power != 12.5 || !dev.Channels[0].On {
		t.Fatalf("cache not updated from MQTT: %+v", dev)
	}
	s.handleMessage(nil, fakeMessage{topic: "shellies/plug/status/switch:0", payload: []byte(`{"id":0,"output":true,"apower":12.5}`)})
	select {
	case ev := <-sc.Events():
		t.Fatalf("repeated status must not be forwarded: %+v", ev)
	default:
	}

	// Input events are never filtered.
	s.handleMessage(nil, fakeMessage{topic: "shellies/plug/status/input:0", payload: []byte(`{"id":0,"state":true}`)})
	s.handleMessage(nil, fakeMessage{topic: "shellies/plug/status/input:0", payload: []byte(`{"id":0,"state":true}`)})
	for i := 0; i < 2; i++ {
		select {
		case ev := <-sc.Events():
			if ev.Type != "input" {
				t.Fatalf("expected input event, got %+v", ev)
			}
		case <-time.After(time.Second):
			t.Fatal("input events must always be forwarded")
		}
	}
}

type fakeMessage struct {
	topic   string
	payload []byte
}

func (m fakeMessage) Duplicate() bool   { return false }
func (m fakeMessage) Qos() byte         { return 0 }
func (m fakeMessage) Retained() bool    { return false }
func (m fakeMessage) Topic() string     { return m.topic }
func (m fakeMessage) MessageID() uint16 { return 1 }
func (m fakeMessage) Payload() []byte   { return m.payload }
func (m fakeMessage) Ack()              {}

var _ mqtt.Message = fakeMessage{}

func TestHandleMessageRoutesKnownDevicesOnly(t *testing.T) {
	sc := shelly.NewClient()
	defer sc.Close()
	s := NewSubscriber(sc)
	s.topicPrefix = "shellies"
	s.SetDevices([]config.ShellyDeviceConfig{
		{ID: "cfg1", MQTTID: "shellypro2pm-aabb"},
		{ID: "cfg2"}, // mqtt id defaults to config id
	})

	s.handleMessage(nil, fakeMessage{topic: "shellies/shellypro2pm-aabb/status/switch:1", payload: []byte(`{"id":1,"output":true}`)})
	s.handleMessage(nil, fakeMessage{topic: "shellies/cfg2/relay/0", payload: []byte("off")})
	s.handleMessage(nil, fakeMessage{topic: "shellies/unknown/relay/0", payload: []byte("on")})
	s.handleMessage(nil, fakeMessage{topic: "other/cfg2/relay/0", payload: []byte("on")})
	s.handleMessage(nil, fakeMessage{topic: "shellies/cfg2", payload: []byte("on")})

	var events []shelly.Event
	timeout := time.After(time.Second)
	for len(events) < 2 {
		select {
		case ev := <-sc.Events():
			events = append(events, ev)
		case <-timeout:
			t.Fatalf("expected 2 events, got %d", len(events))
		}
	}
	if events[0].DeviceID != "cfg1" || events[0].Channel == nil || *events[0].Channel != 1 {
		t.Fatalf("first event wrong: %+v", events[0])
	}
	if events[1].DeviceID != "cfg2" || events[1].Type != "switch" {
		t.Fatalf("second event wrong: %+v", events[1])
	}
	select {
	case ev := <-sc.Events():
		t.Fatalf("unexpected extra event: %+v", ev)
	default:
	}
}

func TestStartDoesNotBlockOnUnreachableBroker(t *testing.T) {
	sc := shelly.NewClient()
	defer sc.Close()
	s := NewSubscriber(sc)

	if err := s.Start(config.MQTTConfig{Enabled: false}); err != nil {
		t.Fatalf("disabled start: %v", err)
	}
	if s.IsConnected() {
		t.Fatal("disabled subscriber must not be connected")
	}
	if err := s.Start(config.MQTTConfig{Enabled: true, Broker: ""}); err == nil {
		t.Fatal("empty broker must be rejected")
	}

	start := time.Now()
	err := s.Start(config.MQTTConfig{Enabled: true, Broker: "tcp://127.0.0.1:1", ClientID: "t", Username: "u", Password: "p"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("Start blocked on an unreachable broker")
	}
	if s.IsConnected() {
		t.Fatal("must not report connected")
	}
	if s.topicPrefix != "shellies" {
		t.Fatalf("default prefix not applied: %q", s.topicPrefix)
	}
	if err := s.Restart(config.MQTTConfig{Enabled: true, Broker: "tcp://127.0.0.1:1", ClientID: "t2", TopicPrefix: "custom"}); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if s.topicPrefix != "custom" {
		t.Fatal("restart did not apply prefix")
	}
	// Disabling via restart drops the client.
	if err := s.Restart(config.MQTTConfig{Enabled: false}); err != nil {
		t.Fatalf("restart disabled: %v", err)
	}
	s.Close()
	s.Close()
}

func TestGen2EdgeCases(t *testing.T) {
	ignored := map[string]string{
		"status/switch:0": "not json",
		"status/weird":    `{}`,
		"status/sys":      `{"a":1}`,
		"status/switch:x": `{"output":true}`,
		"events/rpc":      "nope",
	}
	for topic, payload := range ignored {
		if got := ParseMessage("d", topic, []byte(payload)); len(got) != 0 {
			t.Errorf("%s %q must be ignored, got %+v", topic, payload, got)
		}
	}
	if len(ParseMessage("d", "status/switch:0", []byte(`{"id":0}`))) != 0 {
		t.Fatal("status without values must not emit")
	}
	for _, payload := range []string{
		`{"method":"NotifyEvent","params":{"ts":1}}`,
		`{"method":"NotifyEvent","params":{"events":"bad"}}`,
		`{"method":"NotifyEvent","params":{"events":[{"component":"switch:0","event":"x"}]}}`,
		`{"method":"Other","params":{}}`,
		`{"method":"NotifyStatus","params":{"ts":1,"switch:0":"bad"}}`,
	} {
		if got := ParseMessage("d", "events/rpc", []byte(payload)); len(got) != 0 {
			t.Errorf("rpc %q must be ignored, got %+v", payload, got)
		}
	}

	cases := map[string]string{
		"status/temperature:0": `{"tC":21.56}`,
		"status/humidity:0":    `{"rh":55}`,
		"status/illuminance:0": `{"lux":120}`,
		"status/smoke:0":       `{"alarm":true}`,
		"status/flood:0":       `{"alarm":false}`,
		"status/devicepower:0": `{"battery":{"percent":90}}`,
		"status/em:0":          `{"total_act_power":10,"a_act_power":1,"a_voltage":230,"a_current":0.1}`,
		"status/pm1:0":         `{"apower":5,"voltage":230,"current":0.02}`,
		"status/em1:0":         `{"act_power":7}`,
		"status/light:0":       `{"output":true,"brightness":50,"apower":3}`,
		"status/cct:0":         `{"output":true,"brightness":40,"ct":3000}`,
		"status/rgb:0":         `{"output":true,"brightness":10,"rgb":[1,2,3]}`,
		"status/rgbw:0":        `{"output":true,"brightness":10,"rgb":[1,2,3],"white":4}`,
		"status/input:1":       `{"state":false,"event":"single_push"}`,
		"status/switch:0":      `{"output":true,"aenergy":{"total":12.5}}`,
	}
	for topic, payload := range cases {
		evs := ParseMessage("d", topic, []byte(payload))
		if len(evs) != 1 || len(evs[0].Props) == 0 {
			t.Errorf("%s: expected one event with props, got %+v", topic, evs)
		}
	}

	rgb := ParseMessage("d", "status/rgbw:0", []byte(`{"output":true,"rgb":[1,2,3],"white":4}`))
	if p := props(rgb[0].Props); p["red"] != "1" || p["white"] != "4" || rgb[0].Type != "rgbw" {
		t.Fatalf("rgbw props wrong: %v", p)
	}
	in := ParseMessage("d", "status/input:1", []byte(`{"state":false,"event":"single_push"}`))
	if props(in[0].Props)["push"] != "1" {
		t.Fatalf("input event push code missing: %v", props(in[0].Props))
	}
	sensor := ParseMessage("d", "status/temperature:0", []byte(`{"tC":21.56}`))
	if sensor[0].Channel != nil || props(sensor[0].Props)["temp"] != "21.6" {
		t.Fatalf("sensor event wrong: %+v", sensor[0])
	}
	energy := ParseMessage("d", "status/switch:0", []byte(`{"output":true,"aenergy":{"total":12.5}}`))
	if props(energy[0].Props)["energy"] != "12.5" {
		t.Fatalf("energy prop wrong: %v", props(energy[0].Props))
	}
	full := ParseMessage("d", "events/rpc", []byte(`{"method":"NotifyFullStatus","params":{"ts":1,"switch:0":{"output":false},"temperature:0":{"tC":30}}}`))
	if len(full) != 2 {
		t.Fatalf("NotifyFullStatus should yield 2 events, got %d", len(full))
	}
	if _, ok := pushCode("unknown"); ok {
		t.Fatal("unknown push code must not resolve")
	}
}

func TestGen1EdgeCases(t *testing.T) {
	ignored := map[string]string{
		"roller/0/state":  "open",
		"roller/0/pos":    "x",
		"light/0/status":  "nope",
		"input/0":         "2",
		"input_event/0":   `{"event":""}`,
		"sensor/temp":     "x",
		"sensor/unknown":  "1",
		"relay/0/energy":  "x",
		"relay/0/power":   "x",
		"relay/0/other":   "1",
		"relay/0":         "garbage",
		"online":          "true",
		"input_event/0/x": "{bad",
	}
	for topic, payload := range ignored {
		if got := ParseMessage("d", topic, []byte(payload)); len(got) != 0 {
			t.Errorf("%s %q must be ignored, got %+v", topic, payload, got)
		}
	}
	if len(ParseMessage("d", "light/0/status", []byte(`{}`))) != 0 {
		t.Fatal("light status without values must be ignored")
	}

	energy := ParseMessage("d", "relay/0/energy", []byte("600"))
	if props(energy[0].Props)["energy"] != "10.0" {
		t.Fatalf("energy conversion wrong: %v", props(energy[0].Props))
	}
	for _, topic := range []string{"sensor/humidity", "sensor/battery", "sensor/lux"} {
		if len(ParseMessage("d", topic, []byte("42"))) != 1 {
			t.Errorf("%s not parsed", topic)
		}
	}
	light := ParseMessage("d", "light/1/status", []byte(`{"ison":true,"brightness":70}`))
	if light[0].Channel == nil || *light[0].Channel != 1 || props(light[0].Props)["bri"] != "70" {
		t.Fatalf("light event wrong: %+v", light[0])
	}
	if ParseMessage("d", "relay/x", []byte("on"))[0].Channel != nil {
		t.Fatal("non-numeric channel must be nil")
	}
	if gen1Channel("relay") != nil {
		t.Fatal("topic without channel segment must yield nil")
	}
}
