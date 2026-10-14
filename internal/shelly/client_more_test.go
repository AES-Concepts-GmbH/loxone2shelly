package shelly

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/sbeyeler/loxone2shelly/internal/models"
)

// fakeGen1 emulates a Gen1 Shelly 2.5 in relay mode with optional basic auth.
type fakeGen1 struct {
	mu    sync.Mutex
	paths []string
}

func (f *fakeGen1) server(t *testing.T, password string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if password != "" {
			u, p, ok := r.BasicAuth()
			if !ok || u != "admin" || p != password {
				w.Header().Set("WWW-Authenticate", `Basic realm="shelly"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.RequestURI())
		f.mu.Unlock()
		switch {
		case r.URL.Path == "/status":
			_, _ = w.Write([]byte(`{"relays":[{"ison":true},{"ison":false}],"meters":[{"power":2.5,"total":120}],"inputs":[{"input":0}],"update":{"old_version":"v1.14"}}`))
		case strings.HasPrefix(r.URL.Path, "/relay/"), strings.HasPrefix(r.URL.Path, "/light/"),
			strings.HasPrefix(r.URL.Path, "/roller/"), strings.HasPrefix(r.URL.Path, "/color/"):
			_, _ = w.Write([]byte(`{"ison":true}`))
		case r.URL.Path == "/broken":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("kaputt"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeGen1) lastPath() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.paths) == 0 {
		return ""
	}
	return f.paths[len(f.paths)-1]
}

func hostOf(srv *httptest.Server) string { return strings.TrimPrefix(srv.URL, "http://") }

func TestGen1DeviceWithBasicAuth(t *testing.T) {
	f := &fakeGen1{}
	srv := f.server(t, "secret")
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "g1", Name: "G1", IP: hostOf(srv), Generation: 1, Password: "secret"})

	dev, err := c.GetDevice("g1")
	if err != nil || !dev.Online || dev.Type != "switch" || !dev.State.On || dev.State.Power != 2.5 {
		t.Fatalf("gen1 state wrong: %+v %v", dev, err)
	}
	if dev.FirmwareVer != "v1.14" || !dev.Capabilities.MultiChannel {
		t.Fatalf("gen1 metadata wrong: %+v", dev)
	}
	if len(c.GetInputs()) != 1 {
		t.Fatal("gen1 inputs missing")
	}

	on, off := true, false
	ch := 1
	cases := []struct {
		cmd  models.DeviceCommand
		want string
	}{
		{models.DeviceCommand{On: &off, Channel: &ch}, "/relay/1?turn=off"},
		{models.DeviceCommand{On: &on}, "/relay/0?turn=on"},
		{models.DeviceCommand{Position: intPtr(40)}, "/roller/0?go=to_pos&roller_pos=40"},
		{models.DeviceCommand{Brightness: floatPtr(70)}, "/light/0?turn=on&brightness=70"},
		{models.DeviceCommand{Brightness: floatPtr(0)}, "/light/0?turn=off&brightness=0"},
		{models.DeviceCommand{Brightness: floatPtr(30), On: &off}, "/light/0?turn=off&brightness=30"},
		{models.DeviceCommand{Red: intPtr(1), Green: intPtr(2), Blue: intPtr(3), White: intPtr(4), On: &on, Brightness: floatPtr(50)}, "/color/0?turn=on&red=1&green=2&blue=3&white=4&gain=50"},
	}
	for _, tc := range cases {
		if err := c.SetDeviceState("g1", tc.cmd); err != nil {
			t.Fatalf("command %+v: %v", tc.cmd, err)
		}
		if got := f.lastPath(); got != tc.want {
			t.Fatalf("command %+v hit %q, want %q", tc.cmd, got, tc.want)
		}
	}
}

func TestGen1OnUsesCachedTypeForComponent(t *testing.T) {
	var last string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		last = r.URL.RequestURI()
		mu.Unlock()
		if r.URL.Path == "/status" {
			_, _ = w.Write([]byte(`{"lights":[{"ison":true,"brightness":50,"mode":"color","red":1,"green":2,"blue":3,"white":4,"gain":60}],"meters":[{"power":1}],"tmp":{"tC":30,"is_valid":true},"hum":{"value":40},"bat":{"value":77}}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "rgbw2", IP: hostOf(srv), Generation: 1})
	dev, _ := c.GetDevice("rgbw2")
	if dev.Type != "rgbw" || dev.State.Red != 1 || dev.State.Brightness != 60 || dev.State.Temperature == nil || dev.State.Battery == nil {
		t.Fatalf("gen1 colour/sensor parse wrong: %+v", dev)
	}
	on := true
	if err := c.SetDeviceState("rgbw2", models.DeviceCommand{On: &on}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if last != "/color/0?turn=on" {
		t.Fatalf("colour device must switch via /color, got %s", last)
	}
}

func TestGen1WrongPasswordIsOffline(t *testing.T) {
	f := &fakeGen1{}
	srv := f.server(t, "secret")
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "g1", IP: hostOf(srv), Generation: 1, Password: "nope"})
	dev, _ := c.GetDevice("g1")
	if dev.Online {
		t.Fatal("wrong password must report offline")
	}
	// Without a password the 401 surfaces as an HTTP error.
	d := &DeviceInfo{ConfigID: "g1", IP: hostOf(srv), Generation: 1}
	if _, err := c.gen1Get(d, "status"); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
	// With a password but a Basic (non-digest) challenge on a Gen2 device the
	// request fails with "authentication required" instead of retrying forever.
	d2 := &DeviceInfo{ConfigID: "g2", IP: hostOf(srv), Generation: 2, Password: "x"}
	if _, err := c.rpcGet(d2, "status"); err == nil || !strings.Contains(err.Error(), "authentication required") {
		t.Fatalf("expected authentication required, got %v", err)
	}
}

func TestDoRequestErrorPaths(t *testing.T) {
	f := &fakeGen1{}
	srv := f.server(t, "")
	c := NewClient()
	defer c.Close()
	d := &DeviceInfo{ConfigID: "x", IP: hostOf(srv), Generation: 1}
	if _, err := c.gen1Get(d, "broken"); err == nil || !strings.Contains(err.Error(), "HTTP 500") || !strings.Contains(err.Error(), "kaputt") {
		t.Fatalf("expected 500 error with body, got %v", err)
	}
	if _, err := c.gen1Get(d, "missing"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404, got %v", err)
	}

	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("plain")) }))
	defer plain.Close()
	d2 := &DeviceInfo{ConfigID: "y", IP: hostOf(plain), Generation: 2}
	if _, err := c.rpcGet(d2, "Shelly.GetStatus"); err == nil || !strings.Contains(err.Error(), "parsing JSON") {
		t.Fatalf("expected json error, got %v", err)
	}
	if _, err := c.rpcPost(d2, "Switch.Set", map[string]interface{}{"id": 0}); err == nil {
		t.Fatal("expected json error from rpcPost")
	}
	if _, err := c.rpcPost(d2, "Switch.Set", make(chan int)); err == nil || !strings.Contains(err.Error(), "marshaling") {
		t.Fatalf("expected marshal error, got %v", err)
	}
	if _, err := c.doRequest(d2, "GET", "http://[::1]:namedport/x", nil); err == nil {
		t.Fatal("invalid URL must fail")
	}
}

func TestGen2ColourAndLightCommands(t *testing.T) {
	var methods []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/Shelly.GetDeviceInfo":
			_, _ = w.Write([]byte(`{"model":"SNDC-0D4P10WW","app":"PlusRGBWPM"}`))
		case "/rpc/Shelly.GetStatus":
			_, _ = w.Write([]byte(`{"rgbw:0":{"output":true,"brightness":50,"rgb":[10,20,30],"white":40}}`))
		case "/rpc":
			var req struct {
				Method string `json:"method"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			methods = append(methods, req.Method)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"id":1,"result":{}}`))
		}
	}))
	defer srv.Close()
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "rgb", IP: hostOf(srv), Generation: 2})
	dev, _ := c.GetDevice("rgb")
	if dev.Type != "rgbw" || dev.State.Red != 10 || dev.State.White != 40 || dev.Model != "SNDC-0D4P10WW" {
		t.Fatalf("rgbw parse wrong: %+v", dev)
	}

	on := true
	cmds := []models.DeviceCommand{
		{Red: intPtr(1), Green: intPtr(2), Blue: intPtr(3), White: intPtr(4), Brightness: floatPtr(80), On: &on},
		{Red: intPtr(1), Green: intPtr(2), Blue: intPtr(3)},
		{Brightness: floatPtr(20)},
		{ColorTemp: intPtr(4000), Brightness: floatPtr(10)},
		{On: &on},
	}
	for _, cmd := range cmds {
		if err := c.SetDeviceState("rgb", cmd); err != nil {
			t.Fatalf("command %+v: %v", cmd, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	want := "RGBW.Set,RGB.Set,Light.Set,CCT.Set,RGBW.Set"
	if got := strings.Join(methods, ","); got != want {
		t.Fatalf("methods = %s, want %s", got, want)
	}
}

func TestGen2OnFallsBackThroughMethods(t *testing.T) {
	var methods []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/Shelly.GetDeviceInfo":
			_, _ = w.Write([]byte(`{"app":"PlusDimmer"}`))
		case "/rpc/Shelly.GetStatus":
			_, _ = w.Write([]byte(`{"light:0":{"output":false,"brightness":0}}`))
		case "/rpc":
			var req struct {
				Method string `json:"method"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			methods = append(methods, req.Method)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"id":1,"error":{"code":404,"message":"No handler"}}`))
		}
	}))
	defer srv.Close()
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "dim", IP: hostOf(srv), Generation: 2})
	_, _ = c.GetDevice("dim")
	on := true
	err := c.SetDeviceState("dim", models.DeviceCommand{On: &on})
	if err == nil || !strings.Contains(err.Error(), "No handler") {
		t.Fatalf("expected RPC error, got %v", err)
	}
	mu.Lock()
	got := strings.Join(methods, ",")
	mu.Unlock()
	if got != "Light.Set,Switch.Set" {
		t.Fatalf("fallback order wrong: %s", got)
	}
	pos := 10
	if err := c.SetDeviceState("dim", models.DeviceCommand{Position: &pos}); err == nil {
		t.Fatal("cover error must propagate")
	}
	bri := 50.0
	if err := c.SetDeviceState("dim", models.DeviceCommand{Brightness: &bri}); err == nil {
		t.Fatal("light error must propagate")
	}
	if err := c.SetDeviceState("dim", models.DeviceCommand{Red: intPtr(1), Green: intPtr(1), Blue: intPtr(1)}); err == nil {
		t.Fatal("colour error must propagate")
	}
}

// fakeGen2Cover emulates a Shelly Plus 2PM in cover profile and records RPC methods.
func fakeGen2Cover(t *testing.T) (*httptest.Server, func() string) {
	t.Helper()
	var methods []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/Shelly.GetDeviceInfo":
			_, _ = w.Write([]byte(`{"app":"Plus2PM","model":"SNSW-102P16EU"}`))
		case "/rpc/Shelly.GetStatus":
			_, _ = w.Write([]byte(`{"cover:0":{"id":0,"state":"stopped","current_pos":30,"apower":0}}`))
		case "/rpc":
			var req struct {
				Method string                 `json:"method"`
				Params map[string]interface{} `json:"params"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			methods = append(methods, req.Method)
			mu.Unlock()
			if !strings.HasPrefix(req.Method, "Cover.") {
				_, _ = w.Write([]byte(`{"id":1,"error":{"code":404,"message":"No handler for ` + req.Method + `"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":1,"result":null}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() string {
		mu.Lock()
		defer mu.Unlock()
		return strings.Join(methods, ",")
	}
}

func TestGen2CoverOpenCloseStop(t *testing.T) {
	srv, methods := fakeGen2Cover(t)
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "cov", IP: hostOf(srv), Generation: 2})
	dev, _ := c.GetDevice("cov")
	if dev.Type != "roller" || !dev.Capabilities.SupportsRoller || dev.State.Position != 30 {
		t.Fatalf("cover parse wrong: %+v", dev)
	}

	on, off, stop := true, false, true
	for _, cmd := range []models.DeviceCommand{{On: &on}, {On: &off}, {Stop: &stop}, {Position: intPtr(50)}} {
		if err := c.SetDeviceState("cov", cmd); err != nil {
			t.Fatalf("command %+v: %v", cmd, err)
		}
	}
	if got := methods(); got != "Cover.Open,Cover.Close,Cover.Stop,Cover.GoToPosition" {
		t.Fatalf("methods = %s", got)
	}
	if err := c.SetDeviceState("cov", models.DeviceCommand{Stop: new(bool)}); err == nil {
		t.Fatal("stop:false is not a command")
	}
}

func TestGen1RollerOpenCloseStop(t *testing.T) {
	f := &fakeGen1{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.RequestURI())
		f.mu.Unlock()
		if r.URL.Path == "/status" {
			_, _ = w.Write([]byte(`{"relays":[{"ison":false}],"rollers":[{"current_pos":40,"power":0}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"state":"open"}`))
	}))
	defer srv.Close()
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "r1", IP: hostOf(srv), Generation: 1})
	if dev, _ := c.GetDevice("r1"); dev.Type != "roller" {
		t.Fatalf("roller parse wrong: %+v", dev)
	}
	on, off, stop := true, false, true
	for _, tc := range []struct {
		cmd  models.DeviceCommand
		want string
	}{
		{models.DeviceCommand{On: &on}, "/roller/0?go=open"},
		{models.DeviceCommand{On: &off}, "/roller/0?go=close"},
		{models.DeviceCommand{Stop: &stop}, "/roller/0?go=stop"},
	} {
		if err := c.SetDeviceState("r1", tc.cmd); err != nil {
			t.Fatalf("command %+v: %v", tc.cmd, err)
		}
		if got := f.lastPath(); got != tc.want {
			t.Fatalf("command %+v hit %q, want %q", tc.cmd, got, tc.want)
		}
	}
}

// TestGen2TransportErrorDoesNotRetryOtherMethods: only an RPC error from the
// device justifies trying the next component; a network failure returns at once.
func TestGen2TransportErrorDoesNotRetryOtherMethods(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rpc" {
			calls.Add(1)
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("hijack unsupported")
			}
			conn, _, _ := hj.Hijack()
			conn.Close() // connection dropped -> transport error
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "x", IP: hostOf(srv), Generation: 2})
	on := true
	if err := c.SetDeviceState("x", models.DeviceCommand{On: &on}); err == nil || isRPCError(err) {
		t.Fatalf("expected transport error, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("transport error must not fall back, got %d calls", calls.Load())
	}
}

func TestApplyStateMergesIntoCache(t *testing.T) {
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "d", Generation: 2})
	temp := 20.0
	c.mu.Lock()
	c.cache["d"] = &models.Device{
		ID: "d", Type: "switch", Online: true,
		State:    models.DeviceState{On: false, Power: 1, Temperature: &temp},
		Channels: []models.Channel{{Index: 0, Type: "switch"}, {Index: 1, Type: "switch"}},
	}
	c.mu.Unlock()

	if changed, known := c.ApplyState("nope", nil, map[string]interface{}{"on": true}); !changed || known {
		t.Fatal("unknown device must be reported as unknown")
	}
	if changed, known := c.ApplyState("d", nil, nil); changed || known {
		t.Fatal("empty state is a no-op")
	}

	// Device-level change updates state and baseline.
	if changed, known := c.ApplyState("d", nil, map[string]interface{}{"on": true, "temperature": 21.5}); !changed || !known {
		t.Fatal("expected a change")
	}
	dev, _ := c.GetDevice("d")
	if !dev.State.On || *dev.State.Temperature != 21.5 || dev.State.Power != 1 {
		t.Fatalf("merge wrong: %+v", dev.State)
	}
	if changed, _ := c.ApplyState("d", nil, map[string]interface{}{"on": true}); changed {
		t.Fatal("same value must not count as change")
	}

	// Channel 1 only touches that channel; channel 0 mirrors the primary state.
	if changed, _ := c.ApplyState("d", intPtr(1), map[string]interface{}{"on": true, "power": 7.5}); !changed {
		t.Fatal("channel change expected")
	}
	dev, _ = c.GetDevice("d")
	if !dev.Channels[1].On || dev.Channels[1].Power != 7.5 || dev.Channels[0].On || dev.State.Power != 1 {
		t.Fatalf("channel merge wrong: %+v", dev)
	}
	if changed, _ := c.ApplyState("d", intPtr(0), map[string]interface{}{"on": false}); !changed {
		t.Fatal("channel 0 change expected")
	}
	dev, _ = c.GetDevice("d")
	if dev.Channels[0].On || dev.State.On {
		t.Fatalf("channel 0 must mirror the primary state: %+v", dev)
	}
	// Unknown channel and incompatible types leave the cache alone but are forwarded.
	if changed, known := c.ApplyState("d", intPtr(5), map[string]interface{}{"on": true}); !changed || !known {
		t.Fatal("unknown channel must be forwarded")
	}
	if changed, _ := c.ApplyState("d", nil, map[string]interface{}{"on": "yes"}); !changed {
		t.Fatal("incompatible value must be forwarded")
	}
	if dev, _ := c.GetDevice("d"); dev.State.On {
		t.Fatal("failed merge must not alter the cache")
	}

	// The baseline follows the merge, so a poll reporting the same state is quiet.
	c.mu.RLock()
	last := c.lastStates["d"]
	c.mu.RUnlock()
	if last == nil {
		t.Fatal("baseline not set")
	}
	c.emitChanges(dev)
	select {
	case ev := <-c.Events():
		t.Fatalf("poll after MQTT merge must not emit: %+v", ev)
	default:
	}
}

func TestRefreshDoesNotResurrectRemovedDevice(t *testing.T) {
	f := &fakeGen1{}
	srv := f.server(t, "")
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "g1", IP: hostOf(srv), Generation: 1})
	d, _ := c.deviceSnapshot("g1")
	c.RemoveDevice("g1")
	// A poll that started before the removal completes afterwards.
	c.refreshDevice(d)
	if len(c.GetInputs()) != 0 || len(c.GetDevices()) != 0 {
		t.Fatal("removed device must not reappear in caches")
	}
}

func TestGen2StatusFailureAfterInfo(t *testing.T) {
	var statusFails bool
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/Shelly.GetDeviceInfo":
			_, _ = w.Write([]byte(`{"app":"Plus1","model":"M"}`))
		case "/rpc/Shelly.GetStatus":
			mu.Lock()
			fail := statusFails
			mu.Unlock()
			if fail {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"switch:0":{"output":true}}`))
		}
	}))
	defer srv.Close()
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "p", IP: hostOf(srv), Generation: 2})
	if dev, _ := c.GetDevice("p"); !dev.Online {
		t.Fatal("device should be online")
	}
	mu.Lock()
	statusFails = true
	mu.Unlock()
	dev, _ := c.GetDeviceLive("p")
	if dev.Online || dev.Model != "M" {
		t.Fatalf("status failure must mark offline but keep cached info: %+v", dev)
	}
	// Cached inputs survive an offline poll.
	c.inputCache["p"] = []*models.Input{{ID: "x"}}
	_, _ = c.GetDeviceLive("p")
	if len(c.GetInputs()) != 1 {
		t.Fatal("inputs must not be dropped while offline")
	}
}

func TestPollingLifecycle(t *testing.T) {
	var output bool
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/Shelly.GetDeviceInfo":
			_, _ = w.Write([]byte(`{"app":"Plus1"}`))
		case "/rpc/Shelly.GetStatus":
			mu.Lock()
			o := output
			mu.Unlock()
			_, _ = w.Write([]byte(`{"switch:0":{"output":` + boolStr(o) + `}}`))
		}
	}))
	defer srv.Close()

	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "p", IP: hostOf(srv), Generation: 2})

	// Interval semantics before the loop starts.
	c.SetPollInterval(-5)
	if c.currentPollInterval() != 10*time.Second {
		t.Fatalf("negative interval must fall back to default, got %v", c.currentPollInterval())
	}
	c.SetPollInterval(1)
	c.SetPollInterval(1) // unchanged: no reset signal

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.StartPolling(ctx, 1)
		close(done)
	}()
	// Runtime change while the loop runs: the ticker picks it up.
	time.Sleep(100 * time.Millisecond)
	c.SetPollInterval(2)
	c.SetPollInterval(1)

	mu.Lock()
	output = true
	mu.Unlock()

	select {
	case ev := <-c.Events():
		if ev.DeviceID != "p" || ev.Device == nil || !ev.Device.State.On {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("poller did not emit a change event")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("polling did not stop on context cancel")
	}
}

func TestPollingStopsOnClose(t *testing.T) {
	c := NewClient()
	done := make(chan struct{})
	go func() {
		c.StartPolling(context.Background(), 1)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	c.Close()
	c.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("polling did not stop on Close")
	}
}

func TestPushEventDropsWhenFull(t *testing.T) {
	c := NewClient()
	defer c.Close()
	for i := 0; i < cap(c.eventChan)+5; i++ {
		c.PushEvent(Event{DeviceID: "x"})
	}
	if len(c.eventChan) != cap(c.eventChan) {
		t.Fatal("channel should be full, extra events dropped")
	}
}

func TestRemoveDeviceClearsState(t *testing.T) {
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "d", IP: "127.0.0.1:1", Generation: 2})
	c.mu.Lock()
	c.lastStates["d"] = 1
	c.chanStates["d:0"] = 1
	c.chanStates["other:0"] = 1
	c.mu.Unlock()
	c.RemoveDevice("d")
	c.mu.RLock()
	_, hasD := c.chanStates["d:0"]
	_, hasOther := c.chanStates["other:0"]
	c.mu.RUnlock()
	if hasD || !hasOther {
		t.Fatal("channel state cleanup wrong")
	}
	if c.UpdateDevice(DeviceSpec{ID: "d"}) {
		t.Fatal("update of removed device must return false")
	}
	if _, err := c.GetDeviceLive("d"); err == nil {
		t.Fatal("live read of removed device must fail")
	}
	if c.IsConfigured() {
		t.Fatal("client should have no devices")
	}
}

func TestUpdateDeviceWithChangedIPDropsCache(t *testing.T) {
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "d", IP: "127.0.0.1:1", Generation: 2})
	c.mu.Lock()
	c.cache["d"] = &models.Device{ID: "d"}
	c.infoCache["d"] = cachedInfo{model: "old"}
	c.mu.Unlock()
	c.UpdateDevice(DeviceSpec{ID: "d", IP: "127.0.0.1:2", Generation: 2})
	c.mu.RLock()
	_, cached := c.cache["d"]
	_, info := c.infoCache["d"]
	c.mu.RUnlock()
	if cached || info {
		t.Fatal("IP change must invalidate caches")
	}
}

func TestOfflineDevicesAreSkipped(t *testing.T) {
	c := NewClient()
	defer c.Close()
	c.AddDevice(DeviceSpec{ID: "off", IP: "127.0.0.1:1", Generation: 2})
	c.AddDevice(DeviceSpec{ID: "off1", IP: "127.0.0.1:1", Generation: 1})
	devs := c.RefreshAll()
	if len(devs) != 2 || devs[0].Online || devs[1].Online {
		t.Fatalf("expected two offline devices: %+v", devs)
	}
	c.RefreshAll() // exercises the "still unreachable" path
	c.emitChanges(devs[0])
	c.emitChanges(nil)
	if len(c.Events()) != 0 {
		t.Fatal("offline devices must not emit events")
	}
	if len(c.GetInputs()) != 0 {
		t.Fatal("offline devices have no cached inputs")
	}
}

func TestDiscoveryReturnsQuickly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := time.Now()
	if _, err := DiscoverDevices(ctx, 300*time.Millisecond); err != nil {
		t.Skipf("mDNS not available in this environment: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("discovery exceeded its timeout")
	}
}

func TestEntryToDevice(t *testing.T) {
	ip := []net.IP{net.ParseIP("192.168.1.20")}
	if _, ok := entryToDevice(&zeroconf.ServiceEntry{}); ok {
		t.Fatal("entry without address must be skipped")
	}
	e := &zeroconf.ServiceEntry{AddrIPv4: ip, Text: []string{"gen=2", "app=Plus1PM", "junk"}}
	e.Instance = "ShellyPlus1PM-AABB"
	e.Service = "_shelly._tcp"
	dev, ok := entryToDevice(e)
	if !ok || dev.Gen != 2 || dev.Model != "Plus1PM" || dev.IP != "192.168.1.20" || dev.ID != "shellyplus1pm-aabb" {
		t.Fatalf("gen2 entry wrong: %+v %v", dev, ok)
	}
	e2 := &zeroconf.ServiceEntry{AddrIPv4: ip, Text: []string{"gen=1"}}
	e2.Instance = "shelly1-CCDD"
	e2.Service = "_http._tcp"
	dev, ok = entryToDevice(e2)
	if !ok || dev.Gen != 1 {
		t.Fatalf("gen1 entry wrong: %+v %v", dev, ok)
	}
	e3 := &zeroconf.ServiceEntry{AddrIPv4: ip}
	e3.Instance = "printer"
	e3.Service = "_http._tcp"
	if _, ok := entryToDevice(e3); ok {
		t.Fatal("non-shelly http service must be skipped")
	}
}

func TestDigestHelpers(t *testing.T) {
	params := parseAuthParams(`realm="r, with comma", nonce=abc, qop="auth", opaque="o", broken="unterminated`)
	if params["realm"] != "r, with comma" || params["nonce"] != "abc" || params["qop"] != "auth" || params["broken"] != "unterminated" {
		t.Fatalf("parseAuthParams wrong: %v", params)
	}
	if _, ok := parseDigestChallenge(`Basic realm="x"`); ok {
		t.Fatal("basic challenge must not parse as digest")
	}
	if _, ok := parseDigestChallenge(`Digest realm="x"`); ok {
		t.Fatal("digest without nonce must not parse")
	}
	ch, ok := parseDigestChallenge(`Digest realm="x", nonce="n"`)
	if !ok || ch.algorithm != "MD5" {
		t.Fatalf("default algorithm must be MD5: %+v", ch)
	}
	// RFC 2617 example (MD5, qop=auth)
	ch, _ = parseDigestChallenge(`Digest realm="testrealm@host.com", qop="auth,auth-int", nonce="dcd98b7102dd2f0e8b11d0f600bfb0c093", opaque="5ccc069c403ebaf9f0171e9517f40e41"`)
	authz, err := digestAuthorizationWithNonce(ch, "Mufasa", "Circle Of Life", "GET", "/dir/index.html", "0a4f113b", "00000001")
	if err != nil || !strings.Contains(authz, `response="6629fae49393a05397450978507c4ef1"`) || !strings.Contains(authz, `opaque="5ccc`) {
		t.Fatalf("MD5 digest wrong: %s %v", authz, err)
	}
	ch, _ = parseDigestChallenge(`Digest realm="r", nonce="n", algorithm=MD5`)
	if _, err := digestAuthorizationWithNonce(ch, "u", "p", "GET", "/", "c", "00000001"); err != nil {
		t.Fatal(err)
	}
	ch.algorithm = "SHA-512"
	if _, err := digestAuthorizationWithNonce(ch, "u", "p", "GET", "/", "c", "00000001"); err == nil {
		t.Fatal("unsupported algorithm must fail")
	}
	if _, err := digestAuthorization(ch, "u", "p", "GET", "/"); err == nil {
		t.Fatal("unsupported algorithm must fail via random cnonce path too")
	}
}

func TestCloneDeviceIsDeep(t *testing.T) {
	temp := 1.0
	smoke := true
	d := &models.Device{ID: "d", Channels: []models.Channel{{Index: 0}}, State: models.DeviceState{Temperature: &temp, Smoke: &smoke}}
	cp := cloneDevice(d)
	*cp.State.Temperature = 2
	*cp.State.Smoke = false
	cp.Channels[0].On = true
	if *d.State.Temperature != 1 || !*d.State.Smoke || d.Channels[0].On {
		t.Fatal("clone shares memory with original")
	}
	if cloneDevice(nil) != nil {
		t.Fatal("clone of nil must be nil")
	}
}

func TestParseGen2EdgeComponents(t *testing.T) {
	status := mustJSON(t, `{
		"cover:0":{"current_pos":30,"apower":5,"state":"opening"},
		"cover:1":{"current_pos":0,"state":"stopped"},
		"cct:0":{"output":true,"brightness":20,"ct":2700},
		"rgb:0":{"output":true,"red":5,"green":6,"blue":7},
		"em1:0":{"act_power":1,"voltage":230,"current":0.1},
		"em1:1":{"act_power":2,"voltage":231,"current":0.2},
		"em1:2":{"act_power":3,"voltage":232,"current":0.3},
		"em1data:0":{"total_act_energy":10},"em1data:1":{"total_act_energy":20},
		"pm1:0":{"apower":9,"voltage":229,"current":0.04,"aenergy":{"total":100}},
		"illuminance:0":{"lux":500},"smoke:0":{"alarm":false},"flood:0":{"alarm":true},"devicepower:0":{"battery":{"percent":55}}
	}`)
	dev := &models.Device{}
	parseGen2Status(status, dev)
	if !dev.Capabilities.MultiChannel || dev.Capabilities.ChannelCount != 2 || !dev.Channels[0].On || dev.Channels[1].On {
		t.Fatalf("cover channels wrong: %+v", dev.Channels)
	}
	if dev.State.ColorTemp != 2700 || dev.State.Red != 5 || dev.State.PowerC != 3 || dev.State.TotalEnergy != 30 || dev.State.Energy != 100 {
		t.Fatalf("component values wrong: %+v", dev.State)
	}
	if dev.State.Illuminance == nil || dev.State.Flood == nil || !*dev.State.Flood || dev.State.Battery == nil {
		t.Fatalf("sensors wrong: %+v", dev.State)
	}

	// Sensor-only and meter-only type detection
	sensor := &models.Device{}
	parseGen2Status(mustJSON(t, `{"temperature:0":{"tC":1}}`), sensor)
	meter := &models.Device{}
	parseGen2Status(mustJSON(t, `{"em:0":{"total_act_power":1}}`), meter)
	empty := &models.Device{}
	parseGen2Status(mustJSON(t, `{"sys":{}}`), empty)
	if sensor.Type != "sensor" || meter.Type != "meter" || empty.Type != "" {
		t.Fatalf("type detection wrong: %s %s %q", sensor.Type, meter.Type, empty.Type)
	}
}

func TestParseGen1Roller(t *testing.T) {
	status := mustJSON(t, `{"relays":[{"ison":false},{"ison":false}],"rollers":[{"current_pos":45,"power":12}],"tmp":{"tC":99,"is_valid":false}}`)
	dev := &models.Device{}
	parseGen1Status(status, dev)
	if dev.Type != "roller" || dev.State.Position != 45 || dev.State.Power != 12 || !dev.Capabilities.SupportsRoller {
		t.Fatalf("gen1 roller wrong: %+v", dev)
	}
	if dev.State.Temperature != nil {
		t.Fatal("invalid temperature must be ignored")
	}
	if len(parseGen1Inputs(mustJSON(t, `{"inputs":"bad"}`), "d", "D")) != 0 {
		t.Fatal("non-array inputs must yield nothing")
	}
}

func intPtr(i int) *int           { return &i }
func floatPtr(f float64) *float64 { return &f }
