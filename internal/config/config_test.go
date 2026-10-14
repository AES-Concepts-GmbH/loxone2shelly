package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sbeyeler/loxone2shelly/internal/models"
)

func loadTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return path
}

func TestLoadDefaultsWhenMissing(t *testing.T) {
	loadTemp(t, "")
	cfg := Get()
	if cfg.Server.Port != 8080 || cfg.Shelly.PollInterval != 10 {
		t.Fatalf("unexpected defaults: %+v", cfg.Server)
	}
	if cfg.Shelly.Devices == nil || cfg.Mappings == nil {
		t.Fatal("slices must not be nil")
	}
}

func TestLoadMigratesMiniserverID(t *testing.T) {
	loadTemp(t, `
loxone:
  miniservers:
    - id: ms1
      ip: 10.0.0.1
mappings:
  - id: m1
    loxone_id: a
    shelly_id: b
    enabled: true
`)
	if got := Get().Mappings[0].MiniserverID; got != "ms1" {
		t.Fatalf("expected migrated miniserver id, got %q", got)
	}
}

func TestLoadRejectsDuplicateDeviceIDs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(path, []byte("shelly:\n  devices:\n    - id: a\n      ip: 1.1.1.1\n    - id: a\n      ip: 1.1.1.2\n"), 0o600)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestGetReturnsCopy(t *testing.T) {
	loadTemp(t, "")
	a := Get()
	a.Server.Port = 1
	a.Shelly.Devices = append(a.Shelly.Devices, ShellyDeviceConfig{ID: "x", IP: "1.1.1.1"})
	if Get().Server.Port != 8080 || len(Get().Shelly.Devices) != 0 {
		t.Fatal("Get must return an independent copy")
	}
}

func TestUpdateValidatesAndSaves(t *testing.T) {
	path := loadTemp(t, "")
	_, err := Update(func(c *Config) error {
		c.Shelly.Devices = append(c.Shelly.Devices, ShellyDeviceConfig{ID: "d1", IP: "1.1.1.1", Password: "secret"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Update(func(c *Config) error {
		c.Shelly.Devices = append(c.Shelly.Devices, ShellyDeviceConfig{ID: "d1", IP: "1.1.1.2"})
		return nil
	}); err == nil {
		t.Fatal("expected validation error for duplicate id")
	}
	if len(Get().Shelly.Devices) != 1 {
		t.Fatal("failed update must not be applied")
	}

	if err := Save(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config file must be private, got %v", info.Mode().Perm())
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "secret") {
		t.Fatal("saved file must contain the real secret")
	}
}

func TestMaskedAndRestoreSecrets(t *testing.T) {
	loadTemp(t, "")
	_, _ = Update(func(c *Config) error {
		c.MQTT.Password = "mqttpw"
		c.Server.APIToken = "tok"
		c.Loxone.Miniservers = []MiniserverConfig{{ID: "ms1", HTTPPassword: "pw1"}}
		c.Shelly.Devices = []ShellyDeviceConfig{{ID: "d1", IP: "1.1.1.1", Password: "dpw"}}
		return nil
	})
	masked := Get().Masked()
	if masked.MQTT.Password != MaskedSecret || masked.Server.APIToken != MaskedSecret ||
		masked.Loxone.Miniservers[0].HTTPPassword != MaskedSecret || masked.Shelly.Devices[0].Password != MaskedSecret {
		t.Fatalf("secrets not masked: %+v", masked)
	}

	// Round trip: a client sends the masked config back with one changed password.
	incoming := masked
	incoming.Loxone.Miniservers[0].HTTPPassword = "newpw"
	incoming.Shelly.Devices = append(incoming.Shelly.Devices, ShellyDeviceConfig{ID: "d2", IP: "1.1.1.2", Password: MaskedSecret})
	RestoreSecrets(incoming, Get())
	if incoming.MQTT.Password != "mqttpw" || incoming.Server.APIToken != "tok" {
		t.Fatal("masked secrets must be restored")
	}
	if incoming.Loxone.Miniservers[0].HTTPPassword != "newpw" {
		t.Fatal("changed secret must be kept")
	}
	if incoming.Shelly.Devices[0].Password != "dpw" || incoming.Shelly.Devices[1].Password != "" {
		t.Fatalf("device secrets wrong: %+v", incoming.Shelly.Devices)
	}
}

func TestConcurrentAccessIsRaceFree(t *testing.T) {
	loadTemp(t, "")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			_, _ = Update(func(c *Config) error {
				c.Mappings = append(c.Mappings, models.Mapping{ID: string(rune('a' + i)), LoxoneID: "l", ShellyID: "s"})
				return nil
			})
		}(i)
		go func() {
			defer wg.Done()
			_ = Get().Masked()
			_ = GetMappings()
			_ = GetServer()
		}()
		go func() {
			defer wg.Done()
			_ = Save()
		}()
	}
	wg.Wait()
	if len(GetMappings()) != 20 {
		t.Fatalf("expected 20 mappings, got %d", len(GetMappings()))
	}
}

func TestEnvTokenOverridesFile(t *testing.T) {
	t.Setenv("L2S_API_TOKEN", "fromenv")
	path := loadTemp(t, "server:\n  api_token: fromfile\n")
	if GetServer().APIToken != "fromenv" {
		t.Fatal("environment token must win")
	}

	// The environment token is never written to disk; the file keeps its own.
	if err := Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "fromenv") || !strings.Contains(string(data), "api_token: fromfile") {
		t.Fatalf("env token leaked into the file:\n%s", data)
	}
	if GetServer().APIToken != "fromenv" {
		t.Fatal("Save must not change the runtime token")
	}

	// A token changed at runtime is persisted as usual.
	if _, err := Update(func(c *Config) error { c.Server.APIToken = "changed"; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := Save(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), "api_token: changed") {
		t.Fatalf("runtime token not persisted:\n%s", data)
	}

	// Without the variable the file's token is used and persisted as is.
	t.Setenv("L2S_API_TOKEN", "")
	path = loadTemp(t, "server:\n  api_token: plain\n")
	if GetServer().APIToken != "plain" {
		t.Fatal("file token must apply without env override")
	}
	if err := Save(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), "api_token: plain") {
		t.Fatalf("file token lost:\n%s", data)
	}
}

func TestBackupDirDefaultsNextToConfig(t *testing.T) {
	path := loadTemp(t, "")
	if got, want := BackupDir(), filepath.Join(filepath.Dir(path), "backups"); got != want {
		t.Fatalf("BackupDir = %q, want %q", got, want)
	}
}
