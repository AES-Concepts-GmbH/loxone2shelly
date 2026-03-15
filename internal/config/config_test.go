package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// resetSingleton resets the package-level singleton so tests are independent.
func resetSingleton() {
	cfgOnce = sync.Once{}
	cfg = nil
	cfgPath = ""
}

func TestDefaultConfig(t *testing.T) {
	dc := DefaultConfig()

	if dc.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", dc.Server.Port)
	}
	if dc.Server.Host != "0.0.0.0" {
		t.Errorf("expected default host '0.0.0.0', got %q", dc.Server.Host)
	}
	if dc.Shelly.PollInterval != 10 {
		t.Errorf("expected default poll interval 10, got %d", dc.Shelly.PollInterval)
	}
	if dc.MQTT.Broker != "tcp://localhost:1883" {
		t.Errorf("expected default MQTT broker, got %q", dc.MQTT.Broker)
	}
	if dc.Logging.Level != "info" {
		t.Errorf("expected default log level 'info', got %q", dc.Logging.Level)
	}
	if dc.Shelly.Devices == nil {
		t.Error("expected non-nil Shelly.Devices")
	}
	if dc.Loxone.Miniservers == nil {
		t.Error("expected non-nil Loxone.Miniservers")
	}
	if dc.Mappings == nil {
		t.Error("expected non-nil Mappings")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	resetSingleton()
	cfg, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config for missing file")
	}
	// Should use defaults
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoad_ValidYAML(t *testing.T) {
	resetSingleton()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
server:
  port: 9090
  host: "127.0.0.1"
shelly:
  poll_interval: 30
loxone:
  miniservers: []
logging:
  level: "debug"
  format: "console"
`
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host '127.0.0.1', got %q", cfg.Server.Host)
	}
	if cfg.Shelly.PollInterval != 30 {
		t.Errorf("expected poll interval 30, got %d", cfg.Shelly.PollInterval)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level 'debug', got %q", cfg.Logging.Level)
	}
}

func TestLoad_InvalidPollIntervalDefaulted(t *testing.T) {
	resetSingleton()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
shelly:
  poll_interval: 0
`
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Shelly.PollInterval != 10 {
		t.Errorf("expected default poll interval 10, got %d", cfg.Shelly.PollInterval)
	}
}

func TestLoad_MappingMigration(t *testing.T) {
	resetSingleton()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
loxone:
  miniservers:
    - id: "ms1"
      name: "Miniserver 1"
      ip: "192.168.1.10"
mappings:
  - id: "map1"
    loxone_id: "lox1"
    shelly_id: "s1"
    enabled: true
`
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Mappings) == 0 {
		t.Fatal("expected at least one mapping")
	}
	if cfg.Mappings[0].MiniserverID != "ms1" {
		t.Errorf("expected MiniserverID 'ms1', got %q", cfg.Mappings[0].MiniserverID)
	}
}

func TestSaveAndLoad(t *testing.T) {
	resetSingleton()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Load defaults first to initialize singleton
	c, err := Load(path) // file doesn't exist yet
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	c.Server.Port = 7777

	if err := Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	resetSingleton()
	c2, err := Load(path)
	if err != nil {
		t.Fatalf("Load after save failed: %v", err)
	}
	if c2.Server.Port != 7777 {
		t.Errorf("expected port 7777 after save/load, got %d", c2.Server.Port)
	}
}

func TestGetVersion_Default(t *testing.T) {
	appVersion = ""
	v := GetVersion()
	if v != "dev" {
		t.Errorf("expected 'dev', got %q", v)
	}
}

func TestSetAndGetVersion(t *testing.T) {
	SetVersion("1.2.3")
	if v := GetVersion(); v != "1.2.3" {
		t.Errorf("expected '1.2.3', got %q", v)
	}
	appVersion = "" // reset
}

func TestMiniserverConfig_HTTPInsecureTLS(t *testing.T) {
	resetSingleton()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
loxone:
  miniservers:
    - id: "ms1"
      name: "Test"
      ip: "192.168.1.10"
      http_enabled: true
      http_insecure_tls: true
`
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Loxone.Miniservers) == 0 {
		t.Fatal("expected miniserver config")
	}
	if !cfg.Loxone.Miniservers[0].HTTPInsecureTLS {
		t.Error("expected HTTPInsecureTLS=true")
	}
}
