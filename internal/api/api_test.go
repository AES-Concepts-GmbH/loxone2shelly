package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sbeyeler/loxone2shelly/internal/config"
	"github.com/sbeyeler/loxone2shelly/internal/loxone"
	"github.com/sbeyeler/loxone2shelly/internal/shelly"
)

type fakeSub struct {
	mu       sync.Mutex
	devices  int
	restarts int
}

func (f *fakeSub) SetDevices(d []config.ShellyDeviceConfig) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.devices = len(d)
}
func (f *fakeSub) Restart(config.MQTTConfig) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restarts++
	return nil
}
func (f *fakeSub) IsConnected() bool { return false }

func newTestServer(t *testing.T, yaml string) (*httptest.Server, *Deps) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if yaml != "" {
		if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	sc := shelly.NewClient()
	t.Cleanup(sc.Close)
	for _, d := range cfg.Shelly.Devices {
		sc.AddDevice(shelly.DeviceSpec{ID: d.ID, Name: d.Name, IP: d.IP, Generation: d.Generation})
	}
	mm := loxone.NewMappingManager()
	mm.Load(cfg.Mappings)
	udp := loxone.NewUDPSender()
	udp.Configure(cfg.Loxone.Miniservers)
	httpS := loxone.NewHTTPSender()
	mq := loxone.NewMQTTSender()
	deps := Deps{
		Shelly:    sc,
		Mappings:  mm,
		UDP:       udp,
		HTTP:      httpS,
		MQTT:      mq,
		MQTTSub:   &fakeSub{},
		StaticDir: filepath.Join(dir, "no-static"),
	}
	s := NewServer(deps)
	go s.wsHub.Run()
	t.Cleanup(s.wsHub.Stop)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	testServers.Store(ts, s)
	return ts, &deps
}

// testServers maps test HTTP servers to their API server so tests can reach the hub.
var testServers sync.Map

func serverHubFor(t *testing.T, ts *httptest.Server) *WebSocketHub {
	t.Helper()
	v, ok := testServers.Load(ts)
	if !ok {
		t.Fatal("unknown test server")
	}
	return v.(*Server).wsHub
}

func do(t *testing.T, method, url, body string, headers map[string]string) (*http.Response, string) {
	t.Helper()
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp, sb.String()
}

func TestAuthRequiredWhenTokenConfigured(t *testing.T) {
	ts, _ := newTestServer(t, "server:\n  api_token: s3cret\n")

	if resp, _ := do(t, "GET", ts.URL+"/api/devices", "", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", ts.URL+"/api/devices", "", map[string]string{"Authorization": "Bearer s3cret"}); resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with bearer, got %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", ts.URL+"/api/devices", "", map[string]string{"X-API-Key": "s3cret"}); resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with api key, got %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", ts.URL+"/api/devices?token=s3cret", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with query token, got %d", resp.StatusCode)
	}
	// Health stays public and reports that auth is required.
	resp, body := do(t, "GET", ts.URL+"/api/health", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"auth_required":true`) {
		t.Fatalf("health wrong: %d %s", resp.StatusCode, body)
	}
}

func TestConfigSecretsMaskedAndPreserved(t *testing.T) {
	ts, _ := newTestServer(t, `
mqtt:
  enabled: false
  password: topsecret
loxone:
  miniservers:
    - id: ms1
      ip: 10.0.0.1
      port: 7777
      udp_enabled: true
      http_password: mspw
`)
	resp, body := do(t, "GET", ts.URL+"/api/config", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET config: %d", resp.StatusCode)
	}
	if strings.Contains(body, "topsecret") || strings.Contains(body, "mspw") {
		t.Fatalf("secrets leaked: %s", body)
	}

	// Send the masked config back; secrets must survive.
	var cr ConfigResponse
	if err := json.Unmarshal([]byte(body), &cr); err != nil {
		t.Fatal(err)
	}
	cr.Shelly.PollInterval = 30
	payload, _ := json.Marshal(cr)
	if resp, body := do(t, "PUT", ts.URL+"/api/config", string(payload), nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT config: %d %s", resp.StatusCode, body)
	}
	cfg := config.Get()
	if cfg.MQTT.Password != "topsecret" || cfg.Loxone.Miniservers[0].HTTPPassword != "mspw" {
		t.Fatalf("secrets overwritten by mask: %+v", cfg)
	}
	if cfg.Shelly.PollInterval != 30 {
		t.Fatal("poll interval not updated")
	}
}

func TestConfigUpdatePropagatesDevices(t *testing.T) {
	ts, deps := newTestServer(t, "")
	body := `{"server":{"port":8080,"host":"0.0.0.0"},"shelly":{"poll_interval":5,"devices":[{"id":"n1","name":"New","ip":"10.1.1.1","generation":2}]},"loxone":{"miniservers":[]},"mqtt":{"enabled":false,"broker":"tcp://x:1883"},"logging":{"level":"info","format":"json"}}`
	if resp, b := do(t, "PUT", ts.URL+"/api/config", body, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT config: %d %s", resp.StatusCode, b)
	}
	if !deps.Shelly.HasDevice("n1") {
		t.Fatal("device from config update not registered in shelly client")
	}
	sub := deps.MQTTSub.(*fakeSub)
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.devices != 1 || sub.restarts != 1 {
		t.Fatalf("mqtt subscriber not updated: devices=%d restarts=%d", sub.devices, sub.restarts)
	}
}

func TestShellyDeviceCRUD(t *testing.T) {
	ts, deps := newTestServer(t, "")

	if resp, body := do(t, "POST", ts.URL+"/api/shelly/add", `{"id":"bad id","ip":"10.0.0.5"}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid id accepted: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(t, "POST", ts.URL+"/api/shelly/add", `{"id":"d1","ip":"not an ip!"}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid ip accepted: %d %s", resp.StatusCode, body)
	}
	resp, body := do(t, "POST", ts.URL+"/api/shelly/add", `{"id":"d1","ip":"10.0.0.5","mqtt_id":"shellyplus1-abc","password":"pw"}`, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add: %d %s", resp.StatusCode, body)
	}
	if strings.Contains(body, `"pw"`) {
		t.Fatalf("password echoed back: %s", body)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/shelly/add", `{"id":"d1","ip":"10.0.0.6"}`, nil); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate id accepted: %d", resp.StatusCode)
	}

	// Update without mqtt_id must keep the stored one.
	if resp, body := do(t, "PUT", ts.URL+"/api/shelly/d1", `{"name":"Renamed"}`, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", resp.StatusCode, body)
	}
	cfg := config.Get()
	if cfg.Shelly.Devices[0].MQTTID != "shellyplus1-abc" || cfg.Shelly.Devices[0].Name != "Renamed" || cfg.Shelly.Devices[0].Password != "pw" {
		t.Fatalf("partial update clobbered fields: %+v", cfg.Shelly.Devices[0])
	}
	if !deps.Shelly.HasDevice("d1") {
		t.Fatal("device not in shelly client")
	}

	if resp, _ := do(t, "DELETE", ts.URL+"/api/shelly/d1", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	if deps.Shelly.HasDevice("d1") || len(config.Get().Shelly.Devices) != 0 {
		t.Fatal("device not removed")
	}
	if resp, _ := do(t, "DELETE", ts.URL+"/api/shelly/d1", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second delete should be 404, got %d", resp.StatusCode)
	}
}

func TestMappingLoxoneIDUniqueness(t *testing.T) {
	ts, _ := newTestServer(t, "")
	m := `{"name":"A","loxone_id":"lox","shelly_id":"s1","shelly_type":"switch","enabled":true}`
	if resp, body := do(t, "POST", ts.URL+"/api/mappings", m, nil); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/mappings", m, nil); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate loxone_id accepted: %d", resp.StatusCode)
	}
	bad := `{"name":"B","loxone_id":"with space","shelly_id":"s1","enabled":true}`
	if resp, _ := do(t, "POST", ts.URL+"/api/mappings", bad, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid loxone_id accepted: %d", resp.StatusCode)
	}
}

func TestWebhookValidation(t *testing.T) {
	ts, _ := newTestServer(t, `
server:
  webhook_token: hook
shelly:
  devices:
    - id: dev1
      ip: 10.0.0.9
      generation: 2
mappings:
  - id: m1
    loxone_id: lox1
    shelly_id: dev1
    enabled: true
`)
	base := ts.URL + "/api/webhook"
	if resp, _ := do(t, "GET", base+"?device_id=dev1&event=single_push", "", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing webhook token accepted: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", base+"?token=hook&device_id=nope&event=single_push", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown device accepted: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", base+"?token=hook&device_id=dev1&event=rm_rf", "", nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown event accepted: %d", resp.StatusCode)
	}
	resp, body := do(t, "GET", base+"?token=hook&device_id=dev1&event=single_push&channel=0", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"mappings":1`) {
		t.Fatalf("valid webhook failed: %d %s", resp.StatusCode, body)
	}
}

func TestBackupIDValidationAndRoundTrip(t *testing.T) {
	ts, _ := newTestServer(t, "mqtt:\n  enabled: false\n  broker: tcp://b:1883\n  password: pw\n")

	if resp, _ := do(t, "GET", ts.URL+"/api/backups/..%2F..%2Fetc%2Fpasswd/download", "", nil); resp.StatusCode == http.StatusOK {
		t.Fatal("path traversal id must not be served")
	}
	if resp, _ := do(t, "DELETE", ts.URL+"/api/backups/..x/", "", nil); resp.StatusCode == http.StatusOK {
		t.Fatal("invalid id must be rejected")
	}

	resp, body := do(t, "POST", ts.URL+"/api/backups", `{"remark":"before upgrade"}`, nil)
	if resp.StatusCode != http.StatusCreated || !strings.Contains(body, "before upgrade") {
		t.Fatalf("create backup: %d %s", resp.StatusCode, body)
	}
	var meta BackupMeta
	_ = json.Unmarshal([]byte(body), &meta)

	resp, body = do(t, "GET", ts.URL+"/api/backups", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, meta.ID) || !strings.Contains(body, "before upgrade") {
		t.Fatalf("list backups: %d %s", resp.StatusCode, body)
	}

	_, body = do(t, "GET", ts.URL+"/api/backups/"+meta.ID+"/download", "", nil)
	if !strings.Contains(body, `"mqtt"`) || !strings.Contains(body, "tcp://b:1883") {
		t.Fatalf("backup must include the MQTT section: %s", body)
	}

	// Change config, then restore.
	_, _ = config.Update(func(c *config.Config) error { c.MQTT.Broker = "tcp://changed:1883"; return nil })
	if resp, body := do(t, "POST", ts.URL+"/api/backups/"+meta.ID+"/restore", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("restore: %d %s", resp.StatusCode, body)
	}
	if config.Get().MQTT.Broker != "tcp://b:1883" {
		t.Fatal("restore did not apply MQTT config")
	}
	if resp, _ := do(t, "DELETE", ts.URL+"/api/backups/"+meta.ID, "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("delete backup: %d", resp.StatusCode)
	}
}

func TestExportEscapesXML(t *testing.T) {
	ts, _ := newTestServer(t, `
mappings:
  - id: m1
    name: 'Licht "Büro" & Co'
    loxone_id: 'lox<1>'
    shelly_id: dev1
    shelly_type: switch
    enabled: true
`)
	_, body := do(t, "GET", ts.URL+"/api/export/inputs", "", nil)
	if strings.Contains(body, "lox<1>") || !strings.Contains(body, "lox&lt;1&gt;") {
		t.Fatalf("loxone_id not escaped: %s", body)
	}
	_, body = do(t, "GET", ts.URL+"/api/export/outputs", "", nil)
	if strings.Contains(body, "lox<1>") {
		t.Fatalf("loxone_id not escaped in outputs: %s", body)
	}
}

func TestCORSOnlyForAllowedOrigins(t *testing.T) {
	ts, _ := newTestServer(t, "server:\n  cors_origins: [\"http://allowed.local\"]\n")
	resp, _ := do(t, "GET", ts.URL+"/api/health", "", map[string]string{"Origin": "http://evil.local"})
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS header must not be set for unknown origins")
	}
	resp, _ = do(t, "GET", ts.URL+"/api/health", "", map[string]string{"Origin": "http://allowed.local"})
	if resp.Header.Get("Access-Control-Allow-Origin") != "http://allowed.local" {
		t.Fatal("CORS header missing for allowed origin")
	}
}

func TestConcurrentAPIWritesAreRaceFree(t *testing.T) {
	ts, _ := newTestServer(t, "")
	var wg sync.WaitGroup
	cfgBody := `{"server":{"port":8080,"host":"0.0.0.0"},"shelly":{"devices":[],"poll_interval":10},"loxone":{"miniservers":[]},"mqtt":{"enabled":false,"broker":"tcp://x:1883"},"logging":{"level":"info","format":"json"}}`
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			do(t, "POST", ts.URL+"/api/shelly/add", `{"id":"c`+string(rune('a'+i))+`","ip":"10.9.9.`+string(rune('1'+i))+`"}`, nil)
		}(i)
		go func() {
			defer wg.Done()
			do(t, "GET", ts.URL+"/api/config", "", nil)
		}()
		go func() {
			defer wg.Done()
			do(t, "PUT", ts.URL+"/api/config", cfgBody, nil)
		}()
	}
	wg.Wait()
}

func TestLogsLimitIsCapped(t *testing.T) {
	ts, _ := newTestServer(t, "")
	resp, _ := do(t, "GET", ts.URL+"/api/logs?limit=999999", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logs: %d", resp.StatusCode)
	}
}
