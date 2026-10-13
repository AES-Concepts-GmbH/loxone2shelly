package mqttclient

import (
	"fmt"
	"testing"

	"github.com/sbeyeler/loxone2shelly/internal/models"
)

func props(p []models.Prop) map[string]string {
	m := map[string]string{}
	for _, x := range p {
		m[x.Name] = fmt.Sprintf("%v", x.Value)
	}
	return m
}

func TestGen2SwitchStatusKeepsChannel(t *testing.T) {
	evs := ParseMessage("dev", "status/switch:1", []byte(`{"id":1,"output":true,"apower":0,"voltage":230.2}`))
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	ev := evs[0]
	if ev.Type != "switch" || ev.Channel == nil || *ev.Channel != 1 {
		t.Fatalf("channel lost: %+v", ev)
	}
	p := props(ev.Props)
	if p["on"] != "1" || p["power"] != "0.0" || p["voltage"] != "230.2" {
		t.Fatalf("props wrong: %v", p)
	}
	if st := ev.State.(map[string]interface{}); st["on"] != true {
		t.Fatalf("ui state wrong: %v", st)
	}
}

func TestGen2InputIsNotRelayState(t *testing.T) {
	evs := ParseMessage("dev", "status/input:0", []byte(`{"id":0,"state":true}`))
	if len(evs) != 1 || evs[0].Type != "input" {
		t.Fatalf("expected input event, got %+v", evs)
	}
	p := props(evs[0].Props)
	if _, hasOn := p["on"]; hasOn {
		t.Fatal("input state must not be reported as relay 'on'")
	}
	if p["input"] != "1" {
		t.Fatalf("input prop wrong: %v", p)
	}
}

func TestGen2CoverStatusHasNoOnProperty(t *testing.T) {
	evs := ParseMessage("dev", "status/cover:0", []byte(`{"id":0,"current_pos":42,"state":"stopped"}`))
	p := props(evs[0].Props)
	if _, hasOn := p["on"]; hasOn || p["pos"] != "42" {
		t.Fatalf("cover props wrong: %v", p)
	}
}

func TestGen2NotifyStatusMultipleComponents(t *testing.T) {
	payload := `{"src":"shellypro2pm-x","dst":"shellies/shellypro2pm-x/events","method":"NotifyStatus","params":{"ts":1.0,"switch:0":{"id":0,"output":true},"switch:1":{"id":1,"output":false}}}`
	evs := ParseMessage("dev", "events/rpc", []byte(payload))
	if len(evs) != 2 {
		t.Fatalf("expected 2 events (one per component), got %d", len(evs))
	}
	seen := map[int]string{}
	for _, ev := range evs {
		seen[*ev.Channel] = props(ev.Props)["on"]
	}
	if seen[0] != "1" || seen[1] != "0" {
		t.Fatalf("per-channel states wrong: %v", seen)
	}
}

func TestGen2NotifyEventButtonPush(t *testing.T) {
	payload := `{"method":"NotifyEvent","params":{"ts":1.0,"events":[{"component":"input:2","id":2,"event":"double_push","ts":1.0}]}}`
	evs := ParseMessage("dev", "events/rpc", []byte(payload))
	if len(evs) != 1 || evs[0].Type != "input" || *evs[0].Channel != 2 {
		t.Fatalf("unexpected events: %+v", evs)
	}
	p := props(evs[0].Props)
	if p["push"] != "2" || p["event"] != "double_push" {
		t.Fatalf("push props wrong: %v", p)
	}
}

func TestGen1Messages(t *testing.T) {
	relay := ParseMessage("dev", "relay/1", []byte("on"))
	if len(relay) != 1 || *relay[0].Channel != 1 || props(relay[0].Props)["on"] != "1" {
		t.Fatalf("relay wrong: %+v", relay)
	}
	if len(ParseMessage("dev", "relay/1", []byte("garbage"))) != 0 {
		t.Fatal("invalid relay payload must be ignored")
	}

	power := ParseMessage("dev", "relay/0/power", []byte("12.34"))
	if props(power[0].Props)["power"] != "12.3" {
		t.Fatalf("power wrong: %+v", power)
	}

	input := ParseMessage("dev", "input/0", []byte("1"))
	if input[0].Type != "input" || props(input[0].Props)["input"] != "1" {
		t.Fatalf("input wrong: %+v", input)
	}

	ev := ParseMessage("dev", "input_event/0", []byte(`{"event":"L","event_cnt":2}`))
	if props(ev[0].Props)["push"] != "3" {
		t.Fatalf("long push wrong: %+v", ev)
	}

	roller := ParseMessage("dev", "roller/0/pos", []byte("55"))
	if roller[0].Type != "roller" || props(roller[0].Props)["pos"] != "55" {
		t.Fatalf("roller wrong: %+v", roller)
	}

	temp := ParseMessage("dev", "sensor/temperature", []byte("21.46"))
	if temp[0].Type != "sensor" || props(temp[0].Props)["temp"] != "21.5" || temp[0].Channel != nil {
		t.Fatalf("sensor wrong: %+v", temp)
	}

	if len(ParseMessage("dev", "online", []byte("true"))) != 0 {
		t.Fatal("unknown topics must be ignored")
	}
}

func TestRegisterDeviceReplacesOldMQTTID(t *testing.T) {
	s := NewSubscriber(nil)
	s.RegisterDevice("cfg", "old")
	s.RegisterDevice("cfg", "new")
	if _, ok := s.deviceMap["old"]; ok {
		t.Fatal("old mqtt id must be removed")
	}
	if s.deviceMap["new"] != "cfg" {
		t.Fatal("new mqtt id missing")
	}
	s.UnregisterDevice("cfg")
	if len(s.deviceMap) != 0 {
		t.Fatal("unregister must clear the device")
	}
}
