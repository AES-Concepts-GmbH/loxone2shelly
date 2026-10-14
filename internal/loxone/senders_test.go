package loxone

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sbeyeler/loxone2shelly/internal/config"
	"github.com/sbeyeler/loxone2shelly/internal/models"
)

func udpListener(t *testing.T) (net.PacketConn, int) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	return pc, pc.LocalAddr().(*net.UDPAddr).Port
}

func readPacket(t *testing.T, pc net.PacketConn) string {
	t.Helper()
	buf := make([]byte, 512)
	_ = pc.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatalf("udp read: %v", err)
	}
	return string(buf[:n])
}

func TestUDPSender(t *testing.T) {
	pcA, portA := udpListener(t)
	pcB, portB := udpListener(t)

	s := NewUDPSender()
	if s.IsEnabled() {
		t.Fatal("new sender must be disabled")
	}
	s.Configure([]config.MiniserverConfig{
		{ID: "a", Name: "A", IP: "127.0.0.1", Port: portA, UDPEnabled: true},
		{ID: "b", Name: "B", IP: "127.0.0.1", Port: portB, UDPEnabled: true, SendAll: true},
		{ID: "off", Name: "Off", IP: "127.0.0.1", Port: 1, UDPEnabled: false},
		{ID: "bad", Name: "Bad", IP: "not a host", Port: 1, UDPEnabled: true},
	})
	if !s.IsEnabled() || !s.IsEnabledFor("a") || s.IsEnabledFor("off") || s.IsEnabledFor("bad") || !s.HasSendAll() {
		t.Fatal("configuration state wrong")
	}

	s.Send("a", "licht", "on", 1)
	if got := readPacket(t, pcA); got != "licht/on:1" {
		t.Fatalf("got %q", got)
	}
	s.Send("unknown", "licht", "on", 1) // no-op

	s.SendToAll("licht", "power", "12.5")
	if got := readPacket(t, pcB); got != "licht/power:12.5" {
		t.Fatalf("send_all got %q", got)
	}

	// Reconfigure replaces all targets.
	s.Configure(nil)
	if s.IsEnabled() || s.HasSendAll() {
		t.Fatal("reconfigure did not clear targets")
	}
	s.Close()
}

type httpRecorder struct {
	mu       sync.Mutex
	hits     int
	lastPath string
	lastAuth string
}

func (r *httpRecorder) handler(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.hits++
	r.lastPath = req.URL.Path
	r.lastAuth = req.Header.Get("Authorization")
	r.mu.Unlock()
	if strings.Contains(req.URL.Path, "reject") {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (r *httpRecorder) snapshot() (int, string, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hits, r.lastPath, r.lastAuth
}

// waitHits blocks until the recorder has seen n requests (sends are queued).
func (r *httpRecorder) waitHits(t *testing.T, n int) (string, string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if hits, path, auth := r.snapshot(); hits >= n {
			if hits > n {
				t.Fatalf("expected %d requests, got %d", n, hits)
			}
			return path, auth
		}
		time.Sleep(5 * time.Millisecond)
	}
	hits, _, _ := r.snapshot()
	t.Fatalf("timed out waiting for %d requests (got %d)", n, hits)
	return "", ""
}

// target returns the configured target for tests that call sendHTTP directly.
func (s *HTTPSender) target(t *testing.T, id string) *httpTarget {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	tg, ok := s.targets[id]
	if !ok {
		t.Fatalf("target %q not configured", id)
	}
	return tg
}

func TestHTTPSender(t *testing.T) {
	rec := &httpRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(rec.handler))
	defer srv.Close()

	s := NewHTTPSender()
	if s.IsEnabled() {
		t.Fatal("new sender must be disabled")
	}
	s.Configure([]config.MiniserverConfig{
		{ID: "ms", Name: "MS", HTTPEnabled: true, HTTPURL: srv.URL + "/", HTTPUser: "u", HTTPPassword: "p", SendAll: true},
		{ID: "off", HTTPEnabled: false, HTTPURL: srv.URL},
		{ID: "noaddr", HTTPEnabled: true},
	})
	if !s.IsEnabled() || !s.HasSendAll() {
		t.Fatal("configuration state wrong")
	}

	s.Send("ms", "licht", "on", 1)
	if path, auth := rec.waitHits(t, 1); path != "/dev/sps/io/licht_on/1" || !strings.HasPrefix(auth, "Basic ") {
		t.Fatalf("send wrong: path=%s auth=%s", path, auth)
	}
	s.Send("unknown", "licht", "on", 1)
	s.SendToAll("licht", "event", "a b")
	if path, _ := rec.waitHits(t, 2); path != "/dev/sps/io/licht_event/a b" {
		t.Fatalf("send_all wrong: %s", path)
	}
	// Rejected responses are logged, not fatal.
	s.Send("ms", "reject", "on", 1)
	rec.waitHits(t, 3)

	// IP-only configuration builds an http:// base URL and sends without auth.
	s.Configure([]config.MiniserverConfig{{ID: "ip", HTTPEnabled: true, IP: strings.TrimPrefix(srv.URL, "http://")}})
	s.Send("ip", "x", "on", 0)
	if _, auth := rec.waitHits(t, 4); auth != "" {
		t.Fatalf("ip-only target must send without auth, got %q", auth)
	}
	// Unreachable target: error is logged, call returns.
	s.Configure([]config.MiniserverConfig{{ID: "down", HTTPEnabled: true, HTTPURL: "http://127.0.0.1:1"}})
	s.Send("down", "x", "on", 0)
	if !s.IsEnabled() || s.HasSendAll() {
		t.Fatal("state after reconfigure wrong")
	}
	s.Close()
	if s.IsEnabled() {
		t.Fatal("closed sender must be disabled")
	}
}

// TestHTTPSenderDoesNotBlockOnDeadMiniserver guards the event loop: sending to
// an unreachable target must return immediately and must not delay other targets.
func TestHTTPSenderDoesNotBlockOnDeadMiniserver(t *testing.T) {
	rec := &httpRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(rec.handler))
	defer srv.Close()

	// 10.255.255.1 is unroutable in practice, so the request runs into the timeout.
	s := NewHTTPSender()
	s.Configure([]config.MiniserverConfig{
		{ID: "dead", HTTPEnabled: true, HTTPURL: "http://10.255.255.1:9", SendAll: true},
		{ID: "ok", HTTPEnabled: true, HTTPURL: srv.URL, SendAll: true},
	})
	defer s.Close()

	start := time.Now()
	for i := 0; i < 20; i++ {
		s.SendToAll("licht", "power", i)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("SendToAll blocked for %v", elapsed)
	}
	// The healthy target still receives everything while the dead one hangs.
	rec.waitHits(t, 20)
}

func TestWorkerDropsWhenFull(t *testing.T) {
	block := make(chan struct{})
	w := newWorker("t", 1)
	defer w.close()
	// The first job occupies the goroutine, the second fills the queue; the
	// third must be dropped instead of blocking.
	w.submit(func() { <-block })
	w.submit(func() {})
	if w.submit(func() {}) {
		t.Fatal("full queue must reject jobs")
	}
	close(block)
	w.close()
	w.close() // idempotent
}

func TestHTTPSenderInsecureTLS(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	// sendHTTP is called directly so the outcome is observed synchronously.
	s := NewHTTPSender()
	s.Configure([]config.MiniserverConfig{{ID: "ms", HTTPEnabled: true, HTTPURL: srv.URL}})
	s.sendHTTP(s.target(t, "ms"), "a", "on", 1)
	if hits.Load() != 0 {
		t.Fatal("TLS verification must be enforced by default")
	}
	s.Configure([]config.MiniserverConfig{{ID: "ms", HTTPEnabled: true, HTTPURL: srv.URL, HTTPInsecureTLS: true}})
	s.sendHTTP(s.target(t, "ms"), "a", "on", 1)
	if hits.Load() != 1 {
		t.Fatal("insecure TLS opt-in did not work")
	}
	s.Close()
}

func TestMQTTSenderWithoutBroker(t *testing.T) {
	s := NewMQTTSender()
	if s.IsEnabled() || s.HasSendAll() {
		t.Fatal("new sender must be disabled")
	}

	s.Configure(config.MQTTConfig{Enabled: false}, []config.MiniserverConfig{{ID: "ms", MQTTEnabled: true}})
	if s.IsEnabled() {
		t.Fatal("disabled mqtt must not enable")
	}

	s.Configure(config.MQTTConfig{Enabled: true, Broker: "tcp://127.0.0.1:1", ClientID: "t"}, []config.MiniserverConfig{{ID: "ms"}})
	if s.IsEnabled() || s.HasSendAll() {
		t.Fatal("no targets must not enable")
	}

	start := time.Now()
	s.Configure(config.MQTTConfig{Enabled: true, Broker: "tcp://127.0.0.1:1", ClientID: "t", Username: "u", Password: "p"},
		[]config.MiniserverConfig{
			{ID: "ms", Name: "MS", MQTTEnabled: true, SendAll: true},
			{ID: "ms2", MQTTEnabled: true, MQTTTopicPrefix: "custom"},
		})
	if time.Since(start) > 3*time.Second {
		t.Fatal("Configure blocked on an unreachable broker")
	}
	if s.IsEnabled() {
		t.Fatal("must not report enabled while disconnected")
	}
	if !s.HasSendAll() {
		t.Fatal("targets must be registered even while connecting")
	}
	s.Send("ms", "a", "on", 1)
	s.Send("unknown", "a", "on", 1)
	s.SendToAll("a", "on", 1)
	// publish itself skips while disconnected.
	publish(nil, &mqttTarget{name: "x"}, "a", "on", 1)

	// Reconfiguring replaces the client without blocking.
	s.Configure(config.MQTTConfig{Enabled: true, Broker: "tcp://127.0.0.1:1", ClientID: "t2"}, []config.MiniserverConfig{{ID: "ms", MQTTEnabled: true}})
	s.Close()
	s.Close()
	if s.IsEnabled() {
		t.Fatal("closed sender must be disabled")
	}
}

func TestDispatcherWithNilSenders(t *testing.T) {
	mm := NewMappingManager()
	mm.Load([]models.Mapping{{ID: "a", LoxoneID: "l", ShellyID: "d", Enabled: true}})
	d := NewDispatcher(mm, nil, nil, nil)
	d.DispatchProps("d", nil, []models.Prop{{Name: "on", Value: 1}})
	d.DispatchDevice(nil, nil)
	d.DispatchDevice(&models.Device{ID: "d", Type: "switch"}, nil)
	d.DispatchProps("d", nil, nil)
}

func TestDispatcherSendsDeviceLevelAndChannelProps(t *testing.T) {
	mm := NewMappingManager()
	mm.Load([]models.Mapping{
		{ID: "a", LoxoneID: "l", ShellyID: "d", Enabled: true, MiniserverID: "ms"},
		{ID: "b", LoxoneID: "l1", ShellyID: "d", Enabled: true, Channel: intp(1), MiniserverID: "ms"},
	})
	rec := &recordingSender{}
	d := NewDispatcher(mm, rec, nil, nil)
	dev := &models.Device{
		ID: "d", Type: "switch",
		Capabilities: models.Capabilities{SupportsPower: true, MultiChannel: true},
		State:        models.DeviceState{On: true, Power: 3},
		Channels: []models.Channel{
			{Index: 0, Type: "switch", On: true, Power: 3},
			{Index: 1, Type: "switch", On: false},
		},
	}
	d.DispatchDevice(dev, nil)
	d.DispatchDevice(dev, intp(1))
	joined := strings.Join(rec.sent, "\n")
	for _, want := range []string{"ms:l/on:1", "ms:l/power:3.0", "ms:l1/on:0", "ms:l1/power:0.0"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %s", want, joined)
		}
	}
}

func TestMappingManagerTargetsForChannel(t *testing.T) {
	m := NewMappingManager()
	m.Load([]models.Mapping{
		{ID: "dev", LoxoneID: "lox", ShellyID: "d", Enabled: true},
		{ID: "ch1", LoxoneID: "lox_ch1", ShellyID: "d", Enabled: true, Channel: intp(1)},
	})
	ids := func(ts []Target) []string {
		out := make([]string, 0, len(ts))
		for _, x := range ts {
			out = append(out, x.LoxoneID)
		}
		return out
	}
	if got := ids(m.TargetsForChannel("d", intp(1))); fmt.Sprint(got) != "[lox_ch1]" {
		t.Fatalf("channel-specific mapping must win: %v", got)
	}
	if got := ids(m.TargetsForChannel("d", intp(0))); fmt.Sprint(got) != "[lox]" {
		t.Fatalf("channel 0 goes to the device-level mapping: %v", got)
	}
	if got := ids(m.TargetsForChannel("d", nil)); fmt.Sprint(got) != "[lox]" {
		t.Fatalf("device-level update goes to the device-level mapping: %v", got)
	}
	// An unmapped channel reaches the device-level mapping with a suffix, the
	// same way webhook events do.
	if got := ids(m.TargetsForChannel("d", intp(2))); fmt.Sprint(got) != "[lox_2]" {
		t.Fatalf("unmapped channel falls back to suffixed device-level id: %v", got)
	}
	if got := m.TargetsForChannel("other", intp(0)); len(got) != 0 {
		t.Fatalf("no mappings, no targets: %+v", got)
	}

	// The dispatcher applies the same rule for polled state.
	rec := &recordingSender{}
	d := NewDispatcher(m, rec, nil, nil)
	d.DispatchProps("d", intp(2), []models.Prop{{Name: "on", Value: 1}})
	if fmt.Sprint(rec.sent) != "[*:lox_2/on:1]" {
		t.Fatalf("dispatcher sent %v", rec.sent)
	}
}

func TestMappingManagerGetAllAndDisabled(t *testing.T) {
	mm := NewMappingManager()
	mm.Load([]models.Mapping{
		{ID: "a", LoxoneID: "l", ShellyID: "d", Enabled: true},
		{ID: "b", LoxoneID: "x", ShellyID: "d", Enabled: false},
	})
	if got := mm.GetAll(); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("GetAll = %+v", got)
	}
	if mm.GetByLoxoneID("x") != nil {
		t.Fatal("disabled mapping must not resolve")
	}
	if _, _, _, ok := mm.ResolveTarget("x"); ok {
		t.Fatal("disabled mapping must not resolve as target")
	}
	mm.Remove("does-not-exist")
	if len(mm.GetAll()) != 1 {
		t.Fatal("removing unknown id must not change state")
	}
	// Removing the last mapping of a device clears its index entry.
	mm.Remove("a")
	if len(mm.GetByShellyID("d")) != 0 {
		t.Fatal("device index not cleared")
	}
}

func TestPropsFromDeviceColorMeterAndSensors(t *testing.T) {
	dev := &models.Device{
		ID: "c", Type: "rgbw",
		Capabilities: models.Capabilities{SupportsColor: true, SupportsColorTemp: true, SupportsDimming: true, SupportsRoller: true, SupportsEnergyMeter: true},
		State:        models.DeviceState{On: true, Red: 1, Green: 2, Blue: 3, White: 4, ColorTemp: 3000, Position: 50, TotalPower: 100},
	}
	got := propMap(PropsFromDevice(dev, nil))
	for k, v := range map[string]string{"red": "1", "green": "2", "blue": "3", "white": "4", "ct": "3000", "pos": "50", "total_power": "100.0", "bri": "0"} {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	rgb := &models.Device{ID: "r", Type: "rgb", Capabilities: models.Capabilities{SupportsColor: true}}
	if _, hasWhite := propMap(PropsFromDevice(rgb, nil))["white"]; hasWhite {
		t.Fatal("rgb devices must not report white")
	}

	smoke, flood, lux, bat := true, false, 12.0, 80.0
	sensor := &models.Device{ID: "s", Type: "sensor", State: models.DeviceState{Smoke: &smoke, Flood: &flood, Illuminance: &lux, Battery: &bat}}
	got = propMap(PropsFromDevice(sensor, nil))
	if got["smoke"] != "1" || got["flood"] != "0" || got["lux"] != "12" || got["bat"] != "80" {
		t.Fatalf("sensor props wrong: %v", got)
	}

	multi := &models.Device{ID: "m", Channels: []models.Channel{{Index: 0, Type: "light", Brightness: 40}, {Index: 1, Type: "cover", Position: 20}}}
	if propMap(PropsFromDevice(multi, intp(0)))["bri"] != "40" || propMap(PropsFromDevice(multi, intp(1)))["pos"] != "20" {
		t.Fatal("channel light/cover props wrong")
	}
}

func TestWebhookPropsAllEvents(t *testing.T) {
	for ev, want := range map[string]map[string]string{
		"toggle":      {"toggle": "1"},
		"double_push": {"push": "2"},
		"triple_push": {"push": "4"},
		"long_push":   {"push": "3"},
		"btn_down":    {"btn": "1"},
		"btn_up":      {"btn": "0"},
		"on":          {"on": "1"},
		"open":        {},
	} {
		got := propMap(WebhookProps(ev, "7"))
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s: %s = %q, want %q", ev, k, got[k], v)
			}
		}
		if got["value"] != "7" || got["event"] != ev {
			t.Errorf("%s: value/event wrong: %v", ev, got)
		}
	}
}
