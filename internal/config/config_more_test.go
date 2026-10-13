package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sbeyeler/loxone2shelly/internal/models"
)

func TestLoadRejectsInvalidInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cases := map[string]string{
		"server: [unclosed":                                   "",
		"server:\n  port: 70000\n":                            "port",
		"loxone:\n  miniservers:\n    - id: a\n    - id: a\n": "duplicate miniserver",
		"loxone:\n  miniservers:\n    - ip: 1.1.1.1\n":        "without id",
		"shelly:\n  devices:\n    - ip: 1.1.1.1\n":            "without id",
	}
	for content, want := range cases {
		_ = os.WriteFile(path, []byte(content), 0o600)
		_, err := Load(path)
		if err == nil {
			t.Errorf("%q must fail", content)
			continue
		}
		if want != "" && !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v should mention %q", content, err, want)
		}
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("reading a directory must fail")
	}
}

func TestVersionAndPath(t *testing.T) {
	SetVersion("")
	if GetVersion() != "dev" {
		t.Fatal("empty version must read as dev")
	}
	SetVersion("1.2.3")
	if GetVersion() != "1.2.3" {
		t.Fatal("version not stored")
	}
	path := loadTemp(t, "")
	if GetPath() != path {
		t.Fatalf("GetPath = %q, want %q", GetPath(), path)
	}
}

func TestUpdateHelpersAndValidate(t *testing.T) {
	loadTemp(t, "")
	if err := UpdateShelly(ShellyConfig{PollInterval: 0, Devices: []ShellyDeviceConfig{{ID: "a", IP: "1.1.1.1"}}}); err != nil {
		t.Fatal(err)
	}
	cfg := Get()
	if cfg.Shelly.PollInterval != 10 || len(cfg.Shelly.Devices) != 1 {
		t.Fatalf("UpdateShelly not normalized: %+v", cfg.Shelly)
	}
	if err := UpdateMappings([]models.Mapping{{ID: "m", LoxoneID: "l", ShellyID: "a"}}); err != nil {
		t.Fatal(err)
	}
	if len(GetMappings()) != 1 {
		t.Fatal("UpdateMappings not applied")
	}

	bad := &Config{Shelly: ShellyConfig{Devices: []ShellyDeviceConfig{{ID: "x"}, {ID: "x"}}}}
	if err := Validate(bad); err == nil {
		t.Fatal("Validate must reject duplicates")
	}
	good := &Config{}
	if err := Validate(good); err != nil || good.Server.Port != 8080 || good.MQTT.ClientID == "" || good.MQTT.Broker == "" || good.MQTT.TopicPrefix == "" {
		t.Fatalf("Validate must normalize defaults: %v %+v", err, good)
	}
	var nilCfg *Config
	if nilCfg.Clone() != nil {
		t.Fatal("Clone of nil must be nil")
	}
}

func TestUpdateReturnsCopyAndCustomBackupDir(t *testing.T) {
	loadTemp(t, "")
	got, err := Update(func(c *Config) error {
		c.Server.BackupDir = "/tmp/custom-backups"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got.Server.BackupDir = "mutated"
	if BackupDir() != "/tmp/custom-backups" {
		t.Fatalf("BackupDir = %q", BackupDir())
	}
	srv := GetServer()
	srv.CORSOrigins = append(srv.CORSOrigins, "x")
	if len(GetServer().CORSOrigins) != 0 {
		t.Fatal("GetServer must return a copy")
	}
	if _, err := Update(func(c *Config) error { return os.ErrInvalid }); err == nil {
		t.Fatal("Update must propagate the callback error")
	}
}

func TestMaskedLeavesEmptySecretsEmpty(t *testing.T) {
	loadTemp(t, "")
	m := Get().Masked()
	if m.MQTT.Password != "" || m.Server.APIToken != "" || m.Server.WebhookToken != "" {
		t.Fatal("empty secrets must stay empty so the UI shows no placeholder")
	}
	if m.Server.CORSOrigins == nil {
		m.Server.CORSOrigins = []string{}
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deeper", "config.yaml")
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	if err := Save(); err != nil {
		t.Fatalf("Save into missing directory: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config not written: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
}

func TestSaveFailsOnUnwritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	dir := t.TempDir()
	_ = os.Chmod(dir, 0o500)
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if _, err := Load(filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := Save(); err == nil {
		t.Fatal("Save into a read-only directory must fail")
	}
}

func TestLoadDefaultPathWhenEmpty(t *testing.T) {
	mu.Lock()
	cfgPath = ""
	mu.Unlock()
	if GetPath() != "configs/config.yaml" {
		t.Fatalf("default path = %q", GetPath())
	}
	loadTemp(t, "")
}
