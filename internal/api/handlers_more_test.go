package api

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sbeyeler/loxone2shelly/internal/config"
	"github.com/sbeyeler/loxone2shelly/internal/loxone"
	"github.com/sbeyeler/loxone2shelly/internal/shelly"
)

func TestDeviceReadAndControl(t *testing.T) {
	dev := newFakeShelly(t)
	ts, _ := newTestServer(t, "shelly:\n  devices:\n    - id: dev1\n      ip: "+dev.hostPort()+"\n      generation: 2\n")

	resp, body := do(t, "GET", ts.URL+"/api/devices/dev1", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"online":true`) {
		t.Fatalf("get device: %d %s", resp.StatusCode, body)
	}
	if resp, _ := do(t, "GET", ts.URL+"/api/devices/missing", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing device: %d", resp.StatusCode)
	}

	resp, body = do(t, "PUT", ts.URL+"/api/devices/dev1", `{"on":true,"channel":1}`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set device: %d %s", resp.StatusCode, body)
	}
	if !dev.output(1) {
		t.Fatal("channel 1 not switched")
	}
	if !strings.Contains(body, `"channels"`) {
		t.Fatalf("set device should return the refreshed device: %s", body)
	}

	if resp, _ := do(t, "PUT", ts.URL+"/api/devices/dev1", `{"brightness":500}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid brightness: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "PUT", ts.URL+"/api/devices/dev1", `{}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty command: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "PUT", ts.URL+"/api/devices/dev1", `not json`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid json: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "PUT", ts.URL+"/api/devices/missing", `{"on":true}`, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing device set: %d", resp.StatusCode)
	}
	// Device rejects the RPC (channel out of range) -> 502
	if resp, _ := do(t, "PUT", ts.URL+"/api/devices/dev1", `{"on":true,"channel":5}`, nil); resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("device error should map to 502: %d", resp.StatusCode)
	}

	resp, body = do(t, "GET", ts.URL+"/api/devices?refresh=1", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "dev1") {
		t.Fatalf("refresh: %d %s", resp.StatusCode, body)
	}
	resp, body = do(t, "GET", ts.URL+"/api/devices/dev1?refresh=1", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "dev1") {
		t.Fatalf("refresh single: %d %s", resp.StatusCode, body)
	}

	resp, body = do(t, "GET", ts.URL+"/api/inputs", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "dev1_input_0") {
		t.Fatalf("inputs: %d %s", resp.StatusCode, body)
	}

	resp, body = do(t, "POST", ts.URL+"/api/shelly/dev1/test", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"success":true`) {
		t.Fatalf("connection test: %d %s", resp.StatusCode, body)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/shelly/missing/test", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("test missing: %d", resp.StatusCode)
	}
}

func TestConnectionTestReportsOffline(t *testing.T) {
	ts, _ := newTestServer(t, "shelly:\n  devices:\n    - id: off1\n      ip: 127.0.0.1:1\n      generation: 1\n")
	resp, body := do(t, "POST", ts.URL+"/api/shelly/off1/test", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"success":false`) {
		t.Fatalf("offline test: %d %s", resp.StatusCode, body)
	}
	// Commands to an offline device surface as 502.
	if resp, _ := do(t, "PUT", ts.URL+"/api/devices/off1", `{"on":true}`, nil); resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("offline command: %d", resp.StatusCode)
	}
}

func TestMappingUpdateDeleteExportImport(t *testing.T) {
	ts, deps := newTestServer(t, "")

	resp, body := do(t, "POST", ts.URL+"/api/mappings", `{"id":"m1","name":"A","loxone_id":"a","shelly_id":"s","shelly_type":"switch","enabled":true}`, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	do(t, "POST", ts.URL+"/api/mappings", `{"id":"m2","name":"B","loxone_id":"b","shelly_id":"s","enabled":true,"channel":1}`, nil)
	if resp, _ := do(t, "POST", ts.URL+"/api/mappings", `{"id":"m1","name":"dup","loxone_id":"zz","shelly_id":"s","enabled":true}`, nil); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate id accepted: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/mappings", `{"name":"ch","loxone_id":"c","shelly_id":"s","enabled":true,"channel":99}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid channel accepted: %d", resp.StatusCode)
	}

	resp, body = do(t, "PUT", ts.URL+"/api/mappings/m1", `{"name":"A2","loxone_id":"a2","shelly_id":"s","enabled":true}`, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"a2"`) {
		t.Fatalf("update: %d %s", resp.StatusCode, body)
	}
	if deps.Mappings.GetByLoxoneID("a2") == nil || deps.Mappings.GetByLoxoneID("a") != nil {
		t.Fatal("mapping manager not reloaded after update")
	}
	if resp, _ := do(t, "PUT", ts.URL+"/api/mappings/m1", `{"name":"A3","loxone_id":"b","shelly_id":"s","enabled":true}`, nil); resp.StatusCode != http.StatusConflict {
		t.Fatalf("conflicting update accepted: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "PUT", ts.URL+"/api/mappings/missing", `{"loxone_id":"zz","shelly_id":"s"}`, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "PUT", ts.URL+"/api/mappings/m1", `{"loxone_id":"","shelly_id":""}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid update accepted: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "PUT", ts.URL+"/api/mappings/m1", `garbage`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("garbage update accepted: %d", resp.StatusCode)
	}

	resp, body = do(t, "GET", ts.URL+"/api/mappings/export", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Disposition"), "mappings.json") {
		t.Fatalf("export: %d", resp.StatusCode)
	}
	var exported []map[string]interface{}
	if err := json.Unmarshal([]byte(body), &exported); err != nil || len(exported) != 2 {
		t.Fatalf("export body: %v %s", err, body)
	}

	imp := `[{"id":"m1","name":"Imported","loxone_id":"a2","shelly_id":"s","enabled":true},{"id":"m9","name":"New","loxone_id":"n","shelly_id":"s","enabled":true},{"name":"NoID","loxone_id":"q","shelly_id":"s","enabled":true}]`
	resp, body = do(t, "POST", ts.URL+"/api/mappings/import", imp, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"added":2`) || !strings.Contains(body, `"replaced":1`) {
		t.Fatalf("import: %d %s", resp.StatusCode, body)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/mappings/import", `[{"name":"bad","loxone_id":"","shelly_id":""}]`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid import accepted: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/mappings/import", `{"not":"a list"}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("non-list import accepted: %d", resp.StatusCode)
	}

	resp, body = do(t, "GET", ts.URL+"/api/mappings", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Imported") {
		t.Fatalf("list: %d %s", resp.StatusCode, body)
	}
	if resp, _ := do(t, "DELETE", ts.URL+"/api/mappings/m2", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "DELETE", ts.URL+"/api/mappings/m2", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second delete: %d", resp.StatusCode)
	}
	if deps.Mappings.GetByLoxoneID("b") != nil {
		t.Fatal("deleted mapping still active")
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/mappings", `garbage`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("garbage create accepted: %d", resp.StatusCode)
	}
}

func TestUDPTestEndpoint(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	port := pc.LocalAddr().(*net.UDPAddr).Port

	ts, _ := newTestServer(t, "loxone:\n  miniservers:\n    - id: ms1\n      name: MS\n      ip: 127.0.0.1\n      port: "+strconv.Itoa(port)+"\n      udp_enabled: true\n      send_all: true\n")

	if resp, _ := do(t, "POST", ts.URL+"/api/udp/test", `{"property":"on","value":"1"}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing loxone_id accepted: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/udp/test", `{"loxone_id":"x","property":"on","value":"7"}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid on value accepted: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/udp/test", `{"loxone_id":"x","property":"on","value":"1","miniserver_id":"nope"}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown miniserver accepted: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/udp/test", `garbage`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("garbage accepted: %d", resp.StatusCode)
	}

	resp, body := do(t, "POST", ts.URL+"/api/udp/test", `{"loxone_id":"licht","property":"on","value":"1","miniserver_id":"ms1"}`, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"success":true`) {
		t.Fatalf("udp test: %d %s", resp.StatusCode, body)
	}
	if got := readUDP(t, pc); got != "licht/on:1" {
		t.Fatalf("udp payload = %q", got)
	}

	resp, _ = do(t, "POST", ts.URL+"/api/udp/test", `{"loxone_id":"licht","property":"power","value":"12.5"}`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("udp send_all: %d", resp.StatusCode)
	}
	if got := readUDP(t, pc); got != "licht/power:12.5" {
		t.Fatalf("udp payload = %q", got)
	}
}

func TestUDPTestWithoutTargets(t *testing.T) {
	ts, _ := newTestServer(t, "")
	resp, body := do(t, "POST", ts.URL+"/api/udp/test", `{"loxone_id":"x","property":"on","value":"1"}`, nil)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "no UDP targets") {
		t.Fatalf("expected no-targets error, got %d %s", resp.StatusCode, body)
	}
}

func readUDP(t *testing.T, pc net.PacketConn) string {
	t.Helper()
	buf := make([]byte, 1024)
	_ = pc.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatalf("udp read: %v", err)
	}
	return string(buf[:n])
}

func TestValidateUDPValue(t *testing.T) {
	cases := []struct {
		prop, val string
		ok        bool
	}{
		{"on", "1", true}, {"on", "2", false},
		{"bri", "50", true}, {"bri", "-1", false}, {"bri", "x", false},
		{"pos", "100", true}, {"pos", "101", false},
		{"power", "12.5", true}, {"power", "abc", false},
		{"custom", "anything", true}, {"custom", strings.Repeat("x", 65), false},
	}
	for _, c := range cases {
		if got := validateUDPValue(c.prop, c.val); got != c.ok {
			t.Errorf("validateUDPValue(%q,%q) = %v, want %v", c.prop, c.val, got, c.ok)
		}
	}
}

func TestBackupUploadAndErrors(t *testing.T) {
	ts, _ := newTestServer(t, "")

	full := `{"meta":{"version":"0.0.9","created_at":"2026-01-01T00:00:00Z","remark":"old"},"config":{"shelly":{"devices":[{"id":"u1","ip":"10.0.0.1","generation":2}],"poll_interval":15},"loxone":{"miniservers":[]},"mappings":[]}}`
	resp, body := do(t, "POST", ts.URL+"/api/backups/upload", full, nil)
	if resp.StatusCode != http.StatusCreated || !strings.Contains(body, `"remark":"old"`) {
		t.Fatalf("upload full: %d %s", resp.StatusCode, body)
	}
	var meta BackupMeta
	_ = json.Unmarshal([]byte(body), &meta)

	plain := `{"shelly":{"devices":[{"id":"p1","ip":"10.0.0.2","generation":1}],"poll_interval":5},"loxone":{"miniservers":[]},"mappings":[]}`
	resp, body = do(t, "POST", ts.URL+"/api/backups/upload", plain, nil)
	if resp.StatusCode != http.StatusCreated || !strings.Contains(body, `"version":"imported"`) {
		t.Fatalf("upload plain: %d %s", resp.StatusCode, body)
	}

	if resp, _ := do(t, "POST", ts.URL+"/api/backups/upload", `{"hello":"world"}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("garbage upload accepted: %d", resp.StatusCode)
	}
	dup := `{"shelly":{"devices":[{"id":"d","ip":"1.1.1.1"},{"id":"d","ip":"1.1.1.2"}]},"loxone":{"miniservers":[]},"mappings":[]}`
	if resp, body := do(t, "POST", ts.URL+"/api/backups/upload", dup, nil); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "duplicate") {
		t.Fatalf("invalid config upload accepted: %d %s", resp.StatusCode, body)
	}

	if resp, body := do(t, "POST", ts.URL+"/api/backups/"+meta.ID+"/restore", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("restore: %d %s", resp.StatusCode, body)
	}
	cfg := config.Get()
	if len(cfg.Shelly.Devices) != 1 || cfg.Shelly.Devices[0].ID != "u1" || cfg.Shelly.PollInterval != 15 {
		t.Fatalf("restore did not apply: %+v", cfg.Shelly)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/backups/missing/restore", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("restore missing: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", ts.URL+"/api/backups/missing/download", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("download missing: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "DELETE", ts.URL+"/api/backups/missing", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete missing: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", ts.URL+"/api/backups/..x/download", "", nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid id download: %d", resp.StatusCode)
	}
	// Corrupt backup file is reported on restore and tolerated when listing.
	_ = os.WriteFile(filepath.Join(config.BackupDir(), "broken.json"), []byte("{not json"), 0o600)
	if resp, _ := do(t, "POST", ts.URL+"/api/backups/broken/restore", "", nil); resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("broken restore: %d", resp.StatusCode)
	}
	if resp, body := do(t, "GET", ts.URL+"/api/backups", "", nil); resp.StatusCode != http.StatusOK || !strings.Contains(body, "broken") {
		t.Fatalf("list with broken: %d %s", resp.StatusCode, body)
	}
	// Invalid JSON body on create is rejected, empty body is fine.
	if resp, _ := do(t, "POST", ts.URL+"/api/backups", `{bad`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad create body: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/backups", "", nil); resp.StatusCode != http.StatusCreated {
		t.Fatalf("empty create body: %d", resp.StatusCode)
	}
}

func TestSwaggerEndpoints(t *testing.T) {
	ts, _ := newTestServer(t, "")
	resp, body := do(t, "GET", ts.URL+"/api/swagger.json", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "/api/webhook") {
		t.Fatalf("swagger json: %d", resp.StatusCode)
	}
	var spec map[string]interface{}
	if err := json.Unmarshal([]byte(body), &spec); err != nil || spec["openapi"] == nil {
		t.Fatalf("swagger json invalid: %v", err)
	}
	resp, body = do(t, "GET", ts.URL+"/api/swagger", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(strings.ToLower(body), "<html") {
		t.Fatalf("swagger ui: %d", resp.StatusCode)
	}
}

func TestHealthReportsMQTTAndTokens(t *testing.T) {
	ts, _ := newTestServer(t, "server:\n  webhook_token: w\n")
	resp, body := do(t, "GET", ts.URL+"/api/health", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health: %d", resp.StatusCode)
	}
	for _, want := range []string{`"webhook_token_required":true`, `"auth_required":false`, `"mqtt_connected":false`, `"version"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("health missing %s: %s", want, body)
		}
	}
}

func TestLogsEndpointFilters(t *testing.T) {
	ts, _ := newTestServer(t, "")
	resp, body := do(t, "GET", ts.URL+"/api/logs?level=error&search=nothing-here&limit=5", "", nil)
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(body) != "[]" {
		t.Fatalf("logs filter: %d %s", resp.StatusCode, body)
	}
}

func newStaticServer(t *testing.T, staticDir string) *Server {
	t.Helper()
	cfgDir := t.TempDir()
	if _, err := config.Load(filepath.Join(cfgDir, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	sc := shelly.NewClient()
	t.Cleanup(sc.Close)
	return NewServer(Deps{
		Shelly: sc, Mappings: loxone.NewMappingManager(),
		UDP: loxone.NewUDPSender(), HTTP: loxone.NewHTTPSender(), MQTT: loxone.NewMQTTSender(),
		StaticDir: staticDir,
	})
}

func TestSPAHandlerServesIndexForUnknownRoutes(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0o600)
	_ = os.MkdirAll(filepath.Join(dir, "assets"), 0o750)
	_ = os.WriteFile(filepath.Join(dir, "assets", "a.js"), []byte("console.log(1)"), 0o600)

	s := newStaticServer(t, dir)
	go s.wsHub.Run()
	t.Cleanup(s.wsHub.Stop)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	resp, body := do(t, "GET", ts.URL+"/some/client/route", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "app") {
		t.Fatalf("spa fallback: %d %s", resp.StatusCode, body)
	}
	resp, body = do(t, "GET", ts.URL+"/assets/a.js", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "console.log") {
		t.Fatalf("static asset: %d %s", resp.StatusCode, body)
	}
	if resp, _ := do(t, "GET", ts.URL+"/assets/", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("directory falls back to index: %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("X-Frame-Options") == "" {
		t.Fatal("security headers missing")
	}
	// OPTIONS preflight is answered without a body.
	if resp, _ := do(t, "OPTIONS", ts.URL+"/api/devices", "", map[string]string{"Origin": ts.URL}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight: %d", resp.StatusCode)
	}
}

func TestServerStartAndShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	s := newStaticServer(t, filepath.Join(t.TempDir(), "nostatic"))
	errCh := make(chan error, 1)
	go func() { errCh <- s.Start("127.0.0.1", port) }()

	var resp *http.Response
	for i := 0; i < 100; i++ {
		resp, err = http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/api/health")
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server did not start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
}

func TestServerStartFailsOnBusyPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	s := newStaticServer(t, filepath.Join(t.TempDir(), "nostatic"))
	t.Cleanup(s.wsHub.Stop)
	if err := s.Start("127.0.0.1", ln.Addr().(*net.TCPAddr).Port); err == nil {
		t.Fatal("expected bind error")
	}
}

func TestExtractTokenVariants(t *testing.T) {
	r, _ := http.NewRequest("GET", "/x?token=q", nil)
	if extractToken(r) != "q" {
		t.Fatal("query token")
	}
	r.Header.Set("X-API-Key", "k")
	if extractToken(r) != "k" {
		t.Fatal("api key header should win over query")
	}
	r.Header.Set("Authorization", "Bearer b")
	if extractToken(r) != "b" {
		t.Fatal("bearer should win")
	}
	if tokenMatches("x", "") {
		t.Fatal("empty expected token must never match")
	}
}

func TestValidHostPortAndIDs(t *testing.T) {
	for _, ok := range []string{"192.168.1.5", "192.168.1.5:80", "shelly.local", "[::1]:80", "a-b.example"} {
		if !validHostPort(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "has space", "under_score.host", "a..b", strings.Repeat("x", 300)} {
		if validHostPort(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
	if !validID("abc-1.2_x") || validID("") || validID("-leading") || validID("a/b") {
		t.Fatal("validID wrong")
	}
}

func TestConfigUpdateRejectsInvalidInput(t *testing.T) {
	ts, _ := newTestServer(t, "")
	bad := []string{
		`{"shelly":{"devices":[{"id":"bad id","ip":"1.1.1.1"}]}}`,
		`{"shelly":{"devices":[{"id":"ok","ip":"not valid!"}]}}`,
		`{"loxone":{"miniservers":[{"id":"bad id","ip":"1.1.1.1"}]}}`,
		`{"shelly":{"devices":[{"id":"a","ip":"1.1.1.1"},{"id":"a","ip":"1.1.1.2"}]}}`,
		`not json`,
	}
	for _, b := range bad {
		if resp, body := do(t, "PUT", ts.URL+"/api/config", b, nil); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("accepted invalid config %s: %d %s", b, resp.StatusCode, body)
		}
	}
	resp, body := do(t, "PUT", ts.URL+"/api/config", `{"server":{"port":9999,"host":"127.0.0.1"},"shelly":{"devices":[]},"loxone":{"miniservers":[]},"mqtt":{},"logging":{"level":"debug"}}`, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"restart_required":true`) {
		t.Fatalf("port change: %d %s", resp.StatusCode, body)
	}
	// The new address is persisted for the next start; the running listener is unaffected.
	if got := config.Get().Server; got.Port != 9999 || got.Host != "127.0.0.1" {
		t.Fatalf("port/host not persisted: %+v", got)
	}
	// Omitted address keeps the stored one and needs no restart.
	resp, body = do(t, "PUT", ts.URL+"/api/config", `{"server":{},"shelly":{"devices":[]},"loxone":{"miniservers":[]},"mqtt":{},"logging":{"level":"debug"}}`, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"restart_required":false`) || config.Get().Server.Port != 9999 {
		t.Fatalf("omitted address: %d %s port=%d", resp.StatusCode, body, config.Get().Server.Port)
	}
	if resp, _ := do(t, "PUT", ts.URL+"/api/config", `{"server":{"port":70000},"shelly":{"devices":[]},"loxone":{"miniservers":[]},"mqtt":{},"logging":{}}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid port accepted: %d", resp.StatusCode)
	}
}

func TestImportRejectsDuplicateLoxoneIDs(t *testing.T) {
	ts, deps := newTestServer(t, "")
	do(t, "POST", ts.URL+"/api/mappings", `{"id":"m1","name":"A","loxone_id":"a","shelly_id":"s","enabled":true}`, nil)

	// Duplicate against an existing mapping and within the import itself.
	for _, imp := range []string{
		`[{"id":"m2","name":"B","loxone_id":"a","shelly_id":"s","enabled":true}]`,
		`[{"id":"x1","name":"X","loxone_id":"x","shelly_id":"s","enabled":true},{"id":"x2","name":"Y","loxone_id":"x","shelly_id":"s","enabled":true}]`,
	} {
		resp, body := do(t, "POST", ts.URL+"/api/mappings/import", imp, nil)
		if resp.StatusCode != http.StatusConflict || !strings.Contains(body, "loxone_id") {
			t.Fatalf("duplicate import accepted: %d %s", resp.StatusCode, body)
		}
	}
	if len(config.GetMappings()) != 1 || deps.Mappings.GetByLoxoneID("x") != nil {
		t.Fatal("rejected import must not change mappings")
	}
	// Disabled duplicates are fine.
	resp, body := do(t, "POST", ts.URL+"/api/mappings/import", `[{"id":"m3","name":"C","loxone_id":"a","shelly_id":"s","enabled":false}]`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disabled duplicate rejected: %d %s", resp.StatusCode, body)
	}
}

func TestWebhookRejectsUnsafeValues(t *testing.T) {
	ts, _ := newTestServer(t, "shelly:\n  devices:\n    - id: dev1\n      ip: 10.0.0.9\n      generation: 2\n")
	base := ts.URL + "/api/webhook?device_id=dev1&event=single_push"
	for _, v := range []string{"a/b", "x:1", "with space", strings.Repeat("9", 65)} {
		if resp, _ := do(t, "GET", base+"&value="+strings.ReplaceAll(v, " ", "%20"), "", nil); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("value %q accepted: %d", v, resp.StatusCode)
		}
	}
	if resp, _ := do(t, "GET", base+"&value=12.5", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("numeric value rejected: %d", resp.StatusCode)
	}
}

func TestBackupDownloadMasksSecretsAndRestoreKeepsThem(t *testing.T) {
	ts, _ := newTestServer(t, `
server:
  webhook_token: hooksecret
mqtt:
  enabled: false
  broker: tcp://b:1883
  password: brokerpw
shelly:
  devices:
    - id: d1
      ip: 10.0.0.1
      generation: 2
      password: devpw
`)
	resp, body := do(t, "POST", ts.URL+"/api/backups", "", nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	var meta BackupMeta
	_ = json.Unmarshal([]byte(body), &meta)

	_, body = do(t, "GET", ts.URL+"/api/backups/"+meta.ID+"/download", "", nil)
	for _, secret := range []string{"brokerpw", "devpw", "hooksecret"} {
		if strings.Contains(body, secret) {
			t.Fatalf("download leaks %q: %s", secret, body)
		}
	}
	if !strings.Contains(body, config.MaskedSecret) || !strings.Contains(body, "tcp://b:1883") {
		t.Fatalf("download must contain masked secrets and the rest of the config: %s", body)
	}

	// Uploading the masked file and restoring it keeps the stored secrets.
	resp, body = do(t, "POST", ts.URL+"/api/backups/upload", body, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload: %d %s", resp.StatusCode, body)
	}
	var uploaded BackupMeta
	_ = json.Unmarshal([]byte(body), &uploaded)
	if resp, body := do(t, "POST", ts.URL+"/api/backups/"+uploaded.ID+"/restore", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("restore: %d %s", resp.StatusCode, body)
	}
	cfg := config.Get()
	if cfg.MQTT.Password != "brokerpw" || cfg.Shelly.Devices[0].Password != "devpw" || cfg.Server.WebhookToken != "hooksecret" {
		t.Fatalf("restore replaced secrets with the mask: %+v", cfg)
	}
}

func TestMutationsReportSaveFailures(t *testing.T) {
	ts, _ := newTestServer(t, "")
	// Replace the config directory by a file so Save cannot write (also as root).
	dir := filepath.Dir(config.GetPath())
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(dir) })

	resp, body := do(t, "POST", ts.URL+"/api/mappings", `{"id":"m1","name":"A","loxone_id":"a","shelly_id":"s","enabled":true}`, nil)
	if resp.StatusCode != http.StatusInternalServerError || !strings.Contains(body, "could not be saved") {
		t.Fatalf("save failure not reported: %d %s", resp.StatusCode, body)
	}
	resp, body = do(t, "POST", ts.URL+"/api/shelly/add", `{"id":"d1","ip":"10.0.0.5"}`, nil)
	if resp.StatusCode != http.StatusInternalServerError || !strings.Contains(body, "could not be saved") {
		t.Fatalf("save failure not reported: %d %s", resp.StatusCode, body)
	}
}

func TestExportOutputsIsWellFormedXML(t *testing.T) {
	ts, _ := newTestServer(t, `
mappings:
  - id: m1
    name: Dimmer
    loxone_id: dim
    shelly_id: dev1
    shelly_type: dimmer
    enabled: true
  - id: m2
    name: Roller
    loxone_id: roll
    shelly_id: dev2
    shelly_type: roller
    channel: 1
    enabled: true
`)
	_, body := do(t, "GET", ts.URL+"/api/export/outputs", "", nil)
	if strings.Contains(body, ":<v>") || !strings.Contains(body, `"brightness":&lt;v&gt;`) || !strings.Contains(body, `"position":&lt;v&gt;`) {
		t.Fatalf("value placeholder must be escaped: %s", body)
	}
	if !strings.Contains(body, `Title="Roller_ch1 STOP"`) || !strings.Contains(body, `{"stop":true,"channel":1}`) {
		t.Fatalf("roller export misses the stop command: %s", body)
	}
	var doc struct {
		XMLName xml.Name `xml:"VirtualOut"`
		Cmds    []struct {
			Title string `xml:"Title,attr"`
			Body  string `xml:"CmdOnBody,attr"`
		} `xml:"VirtualOutCmd"`
	}
	if err := xml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("export is not well-formed XML: %v", err)
	}
	for _, c := range doc.Cmds {
		if strings.HasSuffix(c.Title, " BRI") && !strings.Contains(c.Body, `"brightness":<v>`) {
			t.Fatalf("decoded body lost the placeholder: %q", c.Body)
		}
	}
	// Inputs stay well-formed as well.
	_, body = do(t, "GET", ts.URL+"/api/export/inputs", "", nil)
	var in struct {
		XMLName xml.Name `xml:"VirtualInUdp"`
	}
	if err := xml.Unmarshal([]byte(body), &in); err != nil {
		t.Fatalf("input export is not well-formed XML: %v", err)
	}
}

func TestSPAIndexGetsBaseHref(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte(`<!doctype html><html><head><title>x</title></head><body>app</body></html>`), 0o600)
	s := newStaticServer(t, dir)
	go s.wsHub.Run()
	t.Cleanup(s.wsHub.Stop)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	resp, body := do(t, "GET", ts.URL+"/", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `<head><base href="/">`) {
		t.Fatalf("root base missing: %d %s", resp.StatusCode, body)
	}
	_, body = do(t, "GET", ts.URL+"/index.html", "", nil)
	if !strings.Contains(body, `<base href="/">`) {
		t.Fatalf("index.html must be served with a base: %s", body)
	}
	_, body = do(t, "GET", ts.URL+"/", "", map[string]string{"X-Ingress-Path": "/api/hassio_ingress/Abc-123_x"})
	if !strings.Contains(body, `<base href="/api/hassio_ingress/Abc-123_x/">`) {
		t.Fatalf("ingress base missing: %s", body)
	}
	_, body = do(t, "GET", ts.URL+"/", "", map[string]string{"X-Ingress-Path": `/bad"><script>`})
	if !strings.Contains(body, `<base href="/">`) || strings.Contains(body, "<script>") {
		t.Fatalf("invalid ingress path must be ignored: %s", body)
	}
	resp, body = do(t, "GET", ts.URL+"/api/health", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"port":`) {
		t.Fatalf("health must report the port: %d %s", resp.StatusCode, body)
	}
}

func TestShellyAddValidationAndMQTTID(t *testing.T) {
	ts, _ := newTestServer(t, "")
	if resp, _ := do(t, "POST", ts.URL+"/api/shelly/add", `garbage`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("garbage add: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/shelly/add", `{"ip":"10.0.0.1","generation":9}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid generation: %d", resp.StatusCode)
	}
	resp, body := do(t, "POST", ts.URL+"/api/shelly/add", `{"ip":"10.0.0.1"}`, nil)
	if resp.StatusCode != http.StatusCreated || !strings.Contains(body, `"generation":2`) || !strings.Contains(body, `"name":"Shelly `) {
		t.Fatalf("defaults not applied: %d %s", resp.StatusCode, body)
	}
	var created map[string]interface{}
	_ = json.Unmarshal([]byte(body), &created)
	id := created["id"].(string)

	if resp, _ := do(t, "PUT", ts.URL+"/api/shelly/"+id, `garbage`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("garbage update: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "PUT", ts.URL+"/api/shelly/"+id, `{"ip":"bad ip"}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad ip update: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "PUT", ts.URL+"/api/shelly/"+id, `{"generation":7}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad generation update: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "PUT", ts.URL+"/api/shelly/missing", `{"name":"x"}`, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: %d", resp.StatusCode)
	}
	resp, body = do(t, "PUT", ts.URL+"/api/shelly/"+id, `{"mqtt_id":"","password":"********","username":"admin","generation":1}`, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"generation":1`) || !strings.Contains(body, `"username":"admin"`) {
		t.Fatalf("update: %d %s", resp.StatusCode, body)
	}
	if config.Get().Shelly.Devices[0].Password != "" {
		t.Fatal("masked password must not be stored literally")
	}
}

func TestBackupIDsAreUniqueWithinASecond(t *testing.T) {
	ts, _ := newTestServer(t, "")
	ids := map[string]bool{}
	for i := 0; i < 3; i++ {
		resp, body := do(t, "POST", ts.URL+"/api/backups", `{"remark":"burst"}`, nil)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create %d: %d %s", i, resp.StatusCode, body)
		}
		var meta BackupMeta
		_ = json.Unmarshal([]byte(body), &meta)
		if ids[meta.ID] {
			t.Fatalf("duplicate backup id %s", meta.ID)
		}
		ids[meta.ID] = true
	}
	resp, body := do(t, "GET", ts.URL+"/api/backups", "", nil)
	var list []BackupMeta
	_ = json.Unmarshal([]byte(body), &list)
	if resp.StatusCode != http.StatusOK || len(list) != 3 {
		t.Fatalf("expected 3 backups, got %d (%s)", len(list), body)
	}
}
