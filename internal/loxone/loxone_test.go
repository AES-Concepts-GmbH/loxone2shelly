package loxone

import (
	"fmt"
	"sync"
	"testing"

	"github.com/sbeyeler/loxone2shelly/internal/models"
)

func intp(i int) *int { return &i }

func propMap(props []models.Prop) map[string]string {
	m := make(map[string]string, len(props))
	for _, p := range props {
		m[p.Name] = fmt.Sprintf("%v", p.Value)
	}
	return m
}

func TestPropsFromDeviceSendsZeroValues(t *testing.T) {
	dev := &models.Device{
		ID:   "d1",
		Type: "switch",
		Capabilities: models.Capabilities{
			SupportsPower:   true,
			SupportsDimming: true,
		},
		State: models.DeviceState{On: false, Power: 0, Brightness: 0},
	}
	got := propMap(PropsFromDevice(dev, nil))
	if got["on"] != "0" || got["power"] != "0.0" || got["bri"] != "0" {
		t.Fatalf("zero values must be sent, got %v", got)
	}
}

func TestPropsFromDeviceRespectsCapabilities(t *testing.T) {
	dev := &models.Device{ID: "d1", Type: "switch", State: models.DeviceState{On: true, Power: 12}}
	got := propMap(PropsFromDevice(dev, nil))
	if _, ok := got["power"]; ok {
		t.Fatal("power must not be sent for devices without power metering")
	}
	if got["on"] != "1" {
		t.Fatalf("on should be 1, got %v", got)
	}
}

func TestPropsFromDeviceChannel(t *testing.T) {
	dev := &models.Device{
		ID:           "d1",
		Type:         "switch",
		Capabilities: models.Capabilities{SupportsPower: true, MultiChannel: true, ChannelCount: 2},
		State:        models.DeviceState{On: true, Power: 10},
		Channels: []models.Channel{
			{Index: 0, Type: "switch", On: true, Power: 10},
			{Index: 1, Type: "switch", On: false, Power: 0},
		},
	}
	got := propMap(PropsFromDevice(dev, intp(1)))
	if got["on"] != "0" || got["power"] != "0.0" {
		t.Fatalf("channel 1 props wrong: %v", got)
	}
	if PropsFromDevice(dev, intp(5)) != nil {
		t.Fatal("unknown channel must yield no props")
	}
}

func TestPropsFromDeviceSensors(t *testing.T) {
	temp, hum := 21.5, 40.0
	dev := &models.Device{ID: "s", Type: "sensor", State: models.DeviceState{Temperature: &temp, Humidity: &hum}}
	got := propMap(PropsFromDevice(dev, nil))
	if got["temp"] != "21.5" || got["hum"] != "40.0" {
		t.Fatalf("sensor props wrong: %v", got)
	}
	if _, ok := got["on"]; ok {
		t.Fatal("sensors must not report an 'on' property")
	}

	meter := &models.Device{ID: "m", Type: "meter", Capabilities: models.Capabilities{SupportsPower: true}}
	if _, ok := propMap(PropsFromDevice(meter, nil))["on"]; ok {
		t.Fatal("meters must not report an 'on' property")
	}
}

func TestParseJSONCommandColor(t *testing.T) {
	p := NewCommandParser()
	cmd, err := p.ParseJSON([]byte(`{"target":"rgbw","params":{"red":255,"green":128,"blue":0,"white":10,"color_temp":4000}}`))
	if err != nil {
		t.Fatal(err)
	}
	dc := p.ToDeviceCommand(cmd, nil)
	if dc.Red == nil || *dc.Red != 255 || dc.Green == nil || *dc.Green != 128 ||
		dc.Blue == nil || *dc.Blue != 0 || dc.White == nil || *dc.White != 10 ||
		dc.ColorTemp == nil || *dc.ColorTemp != 4000 {
		t.Fatalf("color params not mapped: %+v", dc)
	}

	cmd, err = p.ParseJSON([]byte(`{"target":"rgbw","params":{"on":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	dc = p.ToDeviceCommand(cmd, nil)
	if dc.Red != nil || dc.Green != nil || dc.Blue != nil || dc.White != nil || dc.ColorTemp != nil {
		t.Fatalf("absent color params must stay nil: %+v", dc)
	}
}

func TestWebhookPropsDoNotCollideWithRelayState(t *testing.T) {
	got := propMap(WebhookProps("single_push", ""))
	if _, hasOn := got["on"]; hasOn {
		t.Fatal("button events must not set the relay 'on' property")
	}
	if got["push"] != "1" || got["event"] != "single_push" {
		t.Fatalf("unexpected props: %v", got)
	}
	if propMap(WebhookProps("off", ""))["on"] != "0" {
		t.Fatal("relay off event must send on:0")
	}
}

func TestMappingManagerMultipleMappingsPerDevice(t *testing.T) {
	m := NewMappingManager()
	m.Load([]models.Mapping{
		{ID: "a", LoxoneID: "lox_a", ShellyID: "dev", Enabled: true},
		{ID: "b", LoxoneID: "lox_b", ShellyID: "dev", Enabled: true, Channel: intp(1)},
		{ID: "c", LoxoneID: "lox_c", ShellyID: "dev", Enabled: false},
	})

	if got := len(m.GetByShellyID("dev")); got != 2 {
		t.Fatalf("expected 2 enabled mappings, got %d", got)
	}

	// Device-level update reaches only device-level mappings.
	if got := m.TargetsForChannel("dev", nil); len(got) != 1 || got[0].Mapping.ID != "a" {
		t.Fatalf("device-level lookup wrong: %+v", got)
	}
	// Channel 1 goes to its specific mapping.
	if got := m.TargetsForChannel("dev", intp(1)); len(got) != 1 || got[0].Mapping.ID != "b" || got[0].LoxoneID != "lox_b" {
		t.Fatalf("channel lookup wrong: %+v", got)
	}
	// Channel 0 without specific mapping falls back to device level.
	if got := m.TargetsForChannel("dev", intp(0)); len(got) != 1 || got[0].Mapping.ID != "a" || got[0].LoxoneID != "lox_a" {
		t.Fatalf("channel 0 fallback wrong: %+v", got)
	}
	// Channel 2 reaches the device-level mapping under a suffixed ID.
	if got := m.TargetsForChannel("dev", intp(2)); len(got) != 1 || got[0].Mapping.ID != "a" || got[0].LoxoneID != "lox_a_2" {
		t.Fatalf("channel 2 fallback wrong: %+v", got)
	}

	shellyID, _, ch, ok := m.ResolveTarget("lox_b")
	if !ok || shellyID != "dev" || ch == nil || *ch != 1 {
		t.Fatalf("ResolveTarget wrong: %v %v %v", shellyID, ch, ok)
	}

	m.Remove("a")
	if got := m.TargetsForChannel("dev", nil); len(got) != 0 {
		t.Fatal("removed mapping still returned")
	}
	if got := m.GetByShellyID("dev"); len(got) != 1 {
		t.Fatalf("expected 1 mapping left, got %d", len(got))
	}
}

func TestMappingManagerReturnsCopies(t *testing.T) {
	m := NewMappingManager()
	m.Load([]models.Mapping{{ID: "a", LoxoneID: "l", ShellyID: "s", Enabled: true}})
	got := m.GetByLoxoneID("l")
	got.ShellyID = "changed"
	if m.GetByLoxoneID("l").ShellyID != "s" {
		t.Fatal("manager state must not be mutable from outside")
	}
}

type recordingSender struct {
	mu   sync.Mutex
	sent []string
}

func (r *recordingSender) IsEnabled() bool { return true }
func (r *recordingSender) Send(ms, lox, prop string, v interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, fmt.Sprintf("%s:%s/%s:%v", ms, lox, prop, v))
}
func (r *recordingSender) SendToAll(lox, prop string, v interface{}) {
	r.Send("*", lox, prop, v)
}

func TestDispatcherRoutesByChannelAndTransport(t *testing.T) {
	mm := NewMappingManager()
	off := false
	mm.Load([]models.Mapping{
		{ID: "a", LoxoneID: "lox_a", ShellyID: "dev", Enabled: true, MiniserverID: "ms1", FeedbackHTTP: &off},
		{ID: "b", LoxoneID: "lox_b", ShellyID: "dev", Enabled: true, Channel: intp(1)},
	})
	udp, httpS := &recordingSender{}, &recordingSender{}
	d := NewDispatcher(mm, udp, httpS, nil)

	d.DispatchProps("dev", nil, []models.Prop{{Name: "on", Value: 1}})
	d.DispatchProps("dev", intp(1), []models.Prop{{Name: "on", Value: 0}})
	d.DispatchProps("unknown", nil, []models.Prop{{Name: "on", Value: 1}})

	wantUDP := []string{"ms1:lox_a/on:1", "*:lox_b/on:0"}
	if fmt.Sprint(udp.sent) != fmt.Sprint(wantUDP) {
		t.Fatalf("udp sent %v, want %v", udp.sent, wantUDP)
	}
	// HTTP disabled for mapping a, so only the channel mapping is sent.
	if fmt.Sprint(httpS.sent) != fmt.Sprint([]string{"*:lox_b/on:0"}) {
		t.Fatalf("http sent %v", httpS.sent)
	}
}

func TestParseTextCommands(t *testing.T) {
	p := NewCommandParser()
	cmd, err := p.ParseText("SET dimmer BRI 80")
	if err != nil {
		t.Fatal(err)
	}
	dc := p.ToDeviceCommand(cmd, intp(2))
	if dc.Brightness == nil || *dc.Brightness != 80 || dc.On == nil || !*dc.On || dc.Channel == nil || *dc.Channel != 2 {
		t.Fatalf("unexpected device command: %+v", dc)
	}

	if _, err := p.ParseText("SET dimmer BRI 150"); err == nil {
		t.Fatal("brightness > 100 must be rejected")
	}
	if _, err := p.ParseText("SET x"); err == nil {
		t.Fatal("short command must be rejected")
	}
	if _, err := p.ParseText("FOO x ON"); err == nil {
		t.Fatal("unknown command type must be rejected")
	}

	q, err := p.ParseText("GET light STATUS")
	if err != nil || q.Type != "query" {
		t.Fatalf("GET parse wrong: %+v %v", q, err)
	}

	// Cover actions: OPEN/CLOSE map onto on/off, STOP sets the stop flag.
	open, _ := p.ParseText("SET roller OPEN")
	closeCmd, _ := p.ParseText("SET roller CLOSE")
	stop, _ := p.ParseText("SET roller STOP")
	if dc := p.ToDeviceCommand(open, nil); dc.On == nil || !*dc.On || dc.Stop != nil {
		t.Fatalf("OPEN wrong: %+v", dc)
	}
	if dc := p.ToDeviceCommand(closeCmd, nil); dc.On == nil || *dc.On {
		t.Fatalf("CLOSE wrong: %+v", dc)
	}
	if dc := p.ToDeviceCommand(stop, nil); dc.Stop == nil || !*dc.Stop || dc.On != nil {
		t.Fatalf("STOP wrong: %+v", dc)
	}
	js, _ := p.ParseJSON([]byte(`{"target":"roller","params":{"stop":true}}`))
	if dc := p.ToDeviceCommand(js, nil); dc.Stop == nil || !*dc.Stop {
		t.Fatalf("JSON stop wrong: %+v", dc)
	}
}

func TestParseJSONCommandPositionAsFloat(t *testing.T) {
	p := NewCommandParser()
	cmd, err := p.ParseJSON([]byte(`{"target":"roller","params":{"position":45,"channel":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	dc := p.ToDeviceCommand(cmd, nil)
	if dc.Position == nil || *dc.Position != 45 || dc.Channel == nil || *dc.Channel != 1 {
		t.Fatalf("unexpected device command: %+v", dc)
	}
	if _, err := p.ParseJSON([]byte(`{"params":{}}`)); err == nil {
		t.Fatal("missing target must be rejected")
	}
}

func TestFeedbackURLEscapesValues(t *testing.T) {
	got := FeedbackURL("http://ms/", "licht", "event", "a b/c?d")
	want := "http://ms/dev/sps/io/licht_event/a%20b%2Fc%3Fd"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
