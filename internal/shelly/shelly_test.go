package shelly

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sbeyeler/loxone2shelly/internal/models"
)

func mustJSON(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestParseGen2StatusPro2PM(t *testing.T) {
	status := mustJSON(t, `{
		"switch:0":{"id":0,"output":true,"apower":12.5,"voltage":230.1,"current":0.054,"aenergy":{"total":1234.5}},
		"switch:1":{"id":1,"output":false,"apower":0,"voltage":230.0,"current":0,"aenergy":{"total":10}},
		"input:0":{"id":0,"state":false},"input:1":{"id":1,"state":true},
		"temperature:0":{"id":0,"tC":41.2}
	}`)
	dev := &models.Device{}
	parseGen2Status(status, dev)

	if dev.Type != "switch" || !dev.Capabilities.MultiChannel || dev.Capabilities.ChannelCount != 2 {
		t.Fatalf("type/channels wrong: %+v", dev)
	}
	if !dev.State.On || dev.State.Power != 12.5 || !dev.Capabilities.SupportsPower {
		t.Fatalf("primary state wrong: %+v", dev.State)
	}
	if len(dev.Channels) != 2 || dev.Channels[1].On || dev.Channels[1].Index != 1 {
		t.Fatalf("channels wrong: %+v", dev.Channels)
	}
	if dev.State.Temperature == nil || *dev.State.Temperature != 41.2 {
		t.Fatal("device temperature missing")
	}

	inputs := parseGen2Inputs(status, "dev", "Dev")
	if len(inputs) != 2 || inputs[1].Channel != 1 || inputs[1].State.State == nil || !*inputs[1].State.State {
		t.Fatalf("inputs wrong: %+v", inputs)
	}
}

func TestParseGen2StatusPro3EM(t *testing.T) {
	status := mustJSON(t, `{
		"em:0":{"a_act_power":100.5,"b_act_power":0,"c_act_power":50,"total_act_power":150.5,"a_voltage":230,"b_voltage":231,"c_voltage":229,"a_current":0.5,"b_current":0,"c_current":0.25},
		"emdata:0":{"total_act":99999.9}
	}`)
	dev := &models.Device{}
	parseGen2Status(status, dev)
	if dev.Type != "meter" || !dev.Capabilities.SupportsEnergyMeter {
		t.Fatalf("meter not detected: %+v", dev)
	}
	if dev.State.TotalPower != 150.5 || dev.State.PowerB != 0 || dev.State.TotalEnergy != 99999.9 {
		t.Fatalf("meter values wrong: %+v", dev.State)
	}
}

func TestParseGen2StatusAddonSensorIDs(t *testing.T) {
	status := mustJSON(t, `{"switch:0":{"output":false},"temperature:100":{"tC":5.5},"humidity:101":{"rh":55}}`)
	dev := &models.Device{}
	parseGen2Status(status, dev)
	if dev.State.Temperature == nil || *dev.State.Temperature != 5.5 || dev.State.Humidity == nil {
		t.Fatalf("add-on sensors not parsed: %+v", dev.State)
	}
}

func TestParseGen2StatusInputOnlyDevice(t *testing.T) {
	status := mustJSON(t, `{"input:0":{"state":false},"input:1":{"state":false},"input:2":{"state":true},"input:3":{"state":false}}`)
	dev := &models.Device{}
	parseGen2Status(status, dev)
	if dev.Type != "input" {
		t.Fatalf("expected input type, got %q", dev.Type)
	}
}

func TestParseGen1Status(t *testing.T) {
	status := mustJSON(t, `{
		"relays":[{"ison":true},{"ison":false}],
		"meters":[{"power":5.5,"total":6000},{"power":0,"total":0}],
		"inputs":[{"input":1,"event":"S","event_cnt":3},{"input":0,"event":"","event_cnt":0}],
		"update":{"old_version":"20230913-112003/v1.14.0-gcb84623"}
	}`)
	dev := &models.Device{}
	parseGen1Status(status, dev)
	if dev.Type != "switch" || !dev.State.On || dev.State.Power != 5.5 || dev.State.Energy != 100 {
		t.Fatalf("gen1 state wrong: %+v", dev.State)
	}
	if !dev.Capabilities.MultiChannel || len(dev.Channels) != 2 {
		t.Fatalf("gen1 channels wrong: %+v", dev.Channels)
	}
	if dev.FirmwareVer == "" {
		t.Fatal("firmware missing")
	}
	inputs := parseGen1Inputs(status, "g1", "G1")
	if len(inputs) != 2 || inputs[0].State.Event == nil || *inputs[0].State.Event != "S" || *inputs[0].State.EventCount != 3 {
		t.Fatalf("gen1 inputs wrong: %+v", inputs[0].State)
	}
}

func TestAppToDeviceType(t *testing.T) {
	cases := map[string]string{
		"Plus1PM":      "",
		"Plus2PM":      "",
		"PlusI4":       "input",
		"Pro3EM":       "",
		"PlusRGBWPM":   "rgbw",
		"Plus010V":     "",
		"PlusPlugS":    "switch",
		"PlusHT":       "sensor",
		"WallDisplay":  "",
		"PlusSmoke":    "sensor",
		"Mini1G3":      "",
		"ShellyDimmer": "dimmer",
	}
	for app, want := range cases {
		if got := appToDeviceType(app); got != want {
			t.Errorf("appToDeviceType(%q) = %q, want %q", app, got, want)
		}
	}
}

func TestDigestRFC7616Example(t *testing.T) {
	header := `Digest realm="http-auth@example.org", qop="auth, auth-int", algorithm=SHA-256, nonce="7ypf/xlj9XXwfDPEoM4URrv/xwf94BcCAzFZH4GiTo0v", opaque="FQhe/qaU925kfnzjCev0ciny7QMkPqMAFRtzCUYo5tdS"`
	ch, ok := parseDigestChallenge(header)
	if !ok {
		t.Fatal("challenge not parsed")
	}
	authz, err := digestAuthorizationWithNonce(ch, "Mufasa", "Circle of Life", "GET", "/dir/index.html",
		"f2/wE4q74E6zIJEtWaHKaf5wv/H5QzzpXusqGemxURZJ", "00000001")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(authz, `response="753927fa0e85d155564e2e272a28d1802ca10daf4496794697cf8db5856cb6c1"`) {
		t.Fatalf("wrong digest response: %s", authz)
	}
}

// fakeGen2 simulates a Gen2 device with optional digest authentication.
func fakeGen2(t *testing.T, password string, output *atomic.Bool, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if password != "" {
			authz := r.Header.Get("Authorization")
			if authz == "" {
				w.Header().Set("WWW-Authenticate", `Digest qop="auth", realm="shellyplus1-test", nonce="abc", algorithm=SHA-256`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			ch, _ := parseDigestChallenge(`Digest qop="auth", realm="shellyplus1-test", nonce="abc", algorithm=SHA-256`)
			params := parseAuthParams(strings.TrimPrefix(authz, "Digest "))
			expected, _ := digestAuthorizationWithNonce(ch, "admin", password, r.Method, r.URL.RequestURI(), params["cnonce"], params["nc"])
			if !strings.Contains(expected, `response="`+params["response"]+`"`) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		switch {
		case r.URL.Path == "/rpc/Shelly.GetDeviceInfo":
			_, _ = w.Write([]byte(`{"model":"SNSW-001P16EU","fw_id":"20240101","app":"Plus1PM","gen":2}`))
		case r.URL.Path == "/rpc/Shelly.GetStatus":
			_, _ = w.Write([]byte(`{"switch:0":{"output":` + boolStr(output.Load()) + `,"apower":3.5,"voltage":230,"current":0.01,"aenergy":{"total":1}},"input:0":{"state":false}}`))
		case r.URL.Path == "/rpc" && r.Method == http.MethodPost:
			var req struct {
				Method string                 `json:"method"`
				Params map[string]interface{} `json:"params"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Method != "Switch.Set" {
				_, _ = w.Write([]byte(`{"id":1,"error":{"code":404,"message":"No handler for ` + req.Method + `"}}`))
				return
			}
			if on, ok := req.Params["on"].(bool); ok {
				output.Store(on)
			}
			if id, _ := req.Params["id"].(float64); id != 0 {
				_, _ = w.Write([]byte(`{"id":1,"error":{"code":-105,"message":"Argument 'id', value ` + boolStr(false) + ` not found"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":1,"result":{"was_on":false}}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestClientGen2WithDigestAuthAndCache(t *testing.T) {
	var output atomic.Bool
	var calls atomic.Int32
	srv := fakeGen2(t, "pw", &output, &calls)
	defer srv.Close()

	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "d1", Name: "Test", IP: strings.TrimPrefix(srv.URL, "http://"), Generation: 2, Password: "pw"})

	devices := c.GetDevices()
	if len(devices) != 1 || !devices[0].Online {
		t.Fatalf("device should be online: %+v", devices[0])
	}
	if devices[0].Model != "SNSW-001P16EU" || devices[0].State.Power != 3.5 {
		t.Fatalf("device info wrong: %+v", devices[0])
	}
	if len(c.GetInputs()) != 1 {
		t.Fatal("inputs should be cached from the status call")
	}

	// Second read must come from the cache (no additional device calls).
	before := calls.Load()
	_ = c.GetDevices()
	_, _ = c.GetDevice("d1")
	if calls.Load() != before {
		t.Fatal("GetDevices/GetDevice must not query the device when cached")
	}

	// Commands go through digest auth and the live refresh updates the cache.
	on := true
	if err := c.SetDeviceState("d1", models.DeviceCommand{On: &on}); err != nil {
		t.Fatalf("SetDeviceState: %v", err)
	}
	dev, err := c.GetDeviceLive("d1")
	if err != nil || !dev.State.On {
		t.Fatalf("live state wrong: %+v %v", dev, err)
	}
	cached, _ := c.GetDevice("d1")
	if !cached.State.On {
		t.Fatal("cache not updated after live refresh")
	}

	// A change must emit an event with the full device snapshot.
	select {
	case ev := <-c.Events():
		if ev.DeviceID != "d1" || ev.Device == nil || !ev.Device.State.On {
			t.Fatalf("unexpected event: %+v", ev)
		}
	default:
		t.Fatal("expected a state change event")
	}
}

func TestClientWrongPasswordGoesOffline(t *testing.T) {
	var output atomic.Bool
	var calls atomic.Int32
	srv := fakeGen2(t, "pw", &output, &calls)
	defer srv.Close()

	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "d1", IP: strings.TrimPrefix(srv.URL, "http://"), Generation: 2, Password: "wrong"})
	dev, _ := c.GetDevice("d1")
	if dev.Online {
		t.Fatal("device with wrong password must be reported offline")
	}
}

func TestClientCommandValidation(t *testing.T) {
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "d1", IP: "127.0.0.1:1", Generation: 2})
	bri := 150.0
	if err := c.SetDeviceState("d1", models.DeviceCommand{Brightness: &bri}); err == nil {
		t.Fatal("brightness > 100 must be rejected before contacting the device")
	}
	if err := c.SetDeviceState("d1", models.DeviceCommand{}); err == nil {
		t.Fatal("empty command must be rejected")
	}
	if err := c.SetDeviceState("nope", models.DeviceCommand{}); err == nil {
		t.Fatal("unknown device must be rejected")
	}
}

func TestSyncDevices(t *testing.T) {
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "a", IP: "1.1.1.1", Generation: 2})
	c.AddDevice(DeviceSpec{ID: "b", IP: "1.1.1.2", Generation: 2})
	c.SyncDevices([]DeviceSpec{{ID: "b", IP: "1.1.1.9", Generation: 1}, {ID: "c", IP: "1.1.1.3", Generation: 2}})
	if c.HasDevice("a") || !c.HasDevice("b") || !c.HasDevice("c") {
		t.Fatal("sync did not apply add/remove")
	}
	info, _ := c.deviceSnapshot("b")
	if info.IP != "1.1.1.9" || info.Generation != 1 {
		t.Fatalf("sync did not update device: %+v", info)
	}
}

func TestEmitChangesPerChannel(t *testing.T) {
	c := NewClient()
	defer c.Close()
	mk := func(ch1 bool) *models.Device {
		return &models.Device{ID: "d", Type: "switch", Online: true,
			Capabilities: models.Capabilities{MultiChannel: true, ChannelCount: 2},
			State:        models.DeviceState{On: true},
			Channels: []models.Channel{
				{Index: 0, Type: "switch", On: true},
				{Index: 1, Type: "switch", On: ch1},
			}}
	}
	c.emitChanges(mk(false)) // initial, no events
	c.emitChanges(mk(true))  // channel 1 changed

	var events []Event
	for len(c.Events()) > 0 {
		events = append(events, <-c.Events())
	}
	// Device-level JSON changed (channels are part of the device) plus channel 1.
	var sawChannel bool
	for _, ev := range events {
		if ev.Channel != nil && *ev.Channel == 1 {
			sawChannel = true
			st := ev.State.(map[string]interface{})
			if st["on"] != true {
				t.Fatalf("channel event state wrong: %v", st)
			}
		}
	}
	if !sawChannel {
		t.Fatalf("expected a channel 1 event, got %+v", events)
	}
}
