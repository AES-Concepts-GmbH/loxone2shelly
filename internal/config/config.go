package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/models"
	"gopkg.in/yaml.v3"
)

// MaskedSecret is the placeholder returned by the API in place of stored
// passwords. Clients that send this value back leave the stored secret untouched.
const MaskedSecret = "********"

// Config represents the application configuration
type Config struct {
	Server   ServerConfig     `yaml:"server" json:"server"`
	Shelly   ShellyConfig     `yaml:"shelly" json:"shelly"`
	Loxone   LoxoneConfig     `yaml:"loxone" json:"loxone"`
	MQTT     MQTTConfig       `yaml:"mqtt" json:"mqtt"`
	Logging  LoggingConfig    `yaml:"logging" json:"logging"`
	Mappings []models.Mapping `yaml:"mappings" json:"mappings"`
}

// MQTTConfig holds MQTT broker settings
type MQTTConfig struct {
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	Broker      string `yaml:"broker" json:"broker"`
	Username    string `yaml:"username" json:"username"`
	Password    string `yaml:"password" json:"password"`
	ClientID    string `yaml:"client_id" json:"client_id"`
	TopicPrefix string `yaml:"topic_prefix" json:"topic_prefix"`
}

// ServerConfig holds HTTP server settings
type ServerConfig struct {
	Port int    `yaml:"port" json:"port"`
	Host string `yaml:"host" json:"host"`
	// APIToken protects the REST API and the WebSocket when set. Clients send it
	// as "Authorization: Bearer <token>", "X-API-Key: <token>" or "?token=<token>".
	// It can also be provided via the L2S_API_TOKEN environment variable.
	APIToken string `yaml:"api_token" json:"api_token"`
	// WebhookToken protects /api/webhook when set (query parameter "token").
	WebhookToken string `yaml:"webhook_token" json:"webhook_token"`
	// CORSOrigins lists origins allowed to call the API from a browser.
	// Empty means same-origin only (the embedded UI needs no CORS).
	CORSOrigins []string `yaml:"cors_origins" json:"cors_origins"`
	// BackupDir is the directory for configuration backups. Defaults to a
	// "backups" folder next to the configuration file.
	BackupDir string `yaml:"backup_dir" json:"backup_dir"`
}

// ShellyDeviceConfig holds settings for a single Shelly device
type ShellyDeviceConfig struct {
	ID         string `yaml:"id" json:"id"`
	Name       string `yaml:"name" json:"name"`
	IP         string `yaml:"ip" json:"ip"`
	Generation int    `yaml:"generation" json:"generation"` // 1 or 2
	MQTTID     string `yaml:"mqtt_id" json:"mqtt_id"`       // MQTT device ID (e.g. "shellyplus1-aabbccddee"); defaults to ID
	Username   string `yaml:"username,omitempty" json:"username"`
	Password   string `yaml:"password,omitempty" json:"password"`
}

// ShellyConfig holds Shelly device settings
type ShellyConfig struct {
	Devices      []ShellyDeviceConfig `yaml:"devices" json:"devices"`
	PollInterval int                  `yaml:"poll_interval" json:"poll_interval"` // seconds, default 10
}

// MiniserverConfig holds settings for a single Loxone Miniserver
type MiniserverConfig struct {
	ID              string `yaml:"id" json:"id"`
	Name            string `yaml:"name" json:"name"`
	IP              string `yaml:"ip" json:"ip"`
	Port            int    `yaml:"port" json:"port"`
	UDPEnabled      bool   `yaml:"udp_enabled" json:"udp_enabled"`
	HTTPEnabled     bool   `yaml:"http_enabled" json:"http_enabled"`
	HTTPURL         string `yaml:"http_url" json:"http_url"`
	HTTPUser        string `yaml:"http_user" json:"http_user"`
	HTTPPassword    string `yaml:"http_password" json:"http_password"`
	HTTPInsecureTLS bool   `yaml:"http_insecure_tls" json:"http_insecure_tls"` // skip TLS verification for self-signed Miniserver certificates
	MQTTEnabled     bool   `yaml:"mqtt_enabled" json:"mqtt_enabled"`
	MQTTTopicPrefix string `yaml:"mqtt_topic_prefix" json:"mqtt_topic_prefix"` // e.g. "loxone2shelly"; topic: <prefix>/<loxone_id>/<property>
	SendAll         bool   `yaml:"send_all" json:"send_all"`
}

// LoxoneConfig holds Loxone integration settings
type LoxoneConfig struct {
	Miniservers []MiniserverConfig `yaml:"miniservers" json:"miniservers"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

var (
	cfg        = DefaultConfig()
	cfgPath    string
	mu         sync.RWMutex
	appVersion string
	// envAPIToken is the token injected via L2S_API_TOKEN; it overrides the
	// file at runtime but is never written back. fileAPIToken is what the file
	// contained, so Save keeps the file's own value.
	envAPIToken  string
	fileAPIToken string
)

// SetVersion sets the application version for use in backups
func SetVersion(v string) {
	mu.Lock()
	defer mu.Unlock()
	appVersion = v
}

// GetVersion returns the application version
func GetVersion() string {
	mu.RLock()
	defer mu.RUnlock()
	if appVersion == "" {
		return "dev"
	}
	return appVersion
}

// GetPath returns the current config file path
func GetPath() string {
	mu.RLock()
	defer mu.RUnlock()
	return pathLocked()
}

func pathLocked() string {
	if cfgPath == "" {
		return "configs/config.yaml"
	}
	return cfgPath
}

// BackupDir returns the directory used for configuration backups.
func BackupDir() string {
	mu.RLock()
	defer mu.RUnlock()
	if cfg.Server.BackupDir != "" {
		return cfg.Server.BackupDir
	}
	return filepath.Join(filepath.Dir(pathLocked()), "backups")
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 8080,
			Host: "0.0.0.0",
		},
		Shelly: ShellyConfig{
			Devices:      []ShellyDeviceConfig{},
			PollInterval: 10,
		},
		Loxone: LoxoneConfig{
			Miniservers: []MiniserverConfig{},
		},
		MQTT: MQTTConfig{
			Enabled:     false,
			Broker:      "tcp://localhost:1883",
			ClientID:    "loxone2shelly",
			TopicPrefix: "shellies",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Mappings: []models.Mapping{},
	}
}

// Load reads the configuration from a YAML file. A missing file yields the
// defaults. The result is a copy; use Get or Update for later access.
func Load(path string) (*Config, error) {
	mu.Lock()
	defer mu.Unlock()

	cfgPath = path
	loaded := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		log.Info().Str("path", path).Msg("Config file not found, using defaults")
	} else if err := yaml.Unmarshal(data, loaded); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	normalize(loaded)

	fileAPIToken = loaded.Server.APIToken
	envAPIToken = os.Getenv("L2S_API_TOKEN")
	if envAPIToken != "" {
		loaded.Server.APIToken = envAPIToken
	}

	if err := validate(loaded); err != nil {
		return nil, err
	}

	cfg = loaded
	if data != nil {
		log.Info().Str("path", path).Msg("Configuration loaded")
	}
	return cfg.Clone(), nil
}

// normalize fills defaults and migrates legacy fields.
func normalize(c *Config) {
	if c.Shelly.Devices == nil {
		c.Shelly.Devices = []ShellyDeviceConfig{}
	}
	if c.Shelly.PollInterval <= 0 {
		c.Shelly.PollInterval = 10
	}
	if c.Loxone.Miniservers == nil {
		c.Loxone.Miniservers = []MiniserverConfig{}
	}
	if c.Mappings == nil {
		c.Mappings = []models.Mapping{}
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.MQTT.ClientID == "" {
		c.MQTT.ClientID = "loxone2shelly"
	}
	if c.MQTT.TopicPrefix == "" {
		c.MQTT.TopicPrefix = "shellies"
	}
	if c.MQTT.Broker == "" {
		c.MQTT.Broker = "tcp://localhost:1883"
	}

	// Migrate mappings: assign first miniserver to mappings without miniserver_id
	if len(c.Loxone.Miniservers) > 0 {
		firstID := c.Loxone.Miniservers[0].ID
		for i := range c.Mappings {
			if c.Mappings[i].MiniserverID == "" {
				c.Mappings[i].MiniserverID = firstID
			}
		}
	}
}

// validate performs structural checks that would otherwise surface as
// confusing runtime behaviour.
func validate(c *Config) error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", c.Server.Port)
	}
	seen := make(map[string]struct{}, len(c.Shelly.Devices))
	for _, d := range c.Shelly.Devices {
		if d.ID == "" {
			return fmt.Errorf("shelly device without id (ip=%q)", d.IP)
		}
		if _, dup := seen[d.ID]; dup {
			return fmt.Errorf("duplicate shelly device id %q", d.ID)
		}
		seen[d.ID] = struct{}{}
	}
	ms := make(map[string]struct{}, len(c.Loxone.Miniservers))
	for _, m := range c.Loxone.Miniservers {
		if m.ID == "" {
			return fmt.Errorf("miniserver without id (ip=%q)", m.IP)
		}
		if _, dup := ms[m.ID]; dup {
			return fmt.Errorf("duplicate miniserver id %q", m.ID)
		}
		ms[m.ID] = struct{}{}
	}
	return nil
}

// Validate checks a configuration for structural errors.
func Validate(c *Config) error {
	normalize(c)
	return validate(c)
}

// Clone returns a deep copy of the configuration.
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	cp := *c
	cp.Server.CORSOrigins = append([]string(nil), c.Server.CORSOrigins...)
	cp.Shelly.Devices = append([]ShellyDeviceConfig(nil), c.Shelly.Devices...)
	if cp.Shelly.Devices == nil {
		cp.Shelly.Devices = []ShellyDeviceConfig{}
	}
	cp.Loxone.Miniservers = append([]MiniserverConfig(nil), c.Loxone.Miniservers...)
	if cp.Loxone.Miniservers == nil {
		cp.Loxone.Miniservers = []MiniserverConfig{}
	}
	cp.Mappings = make([]models.Mapping, len(c.Mappings))
	for i, m := range c.Mappings {
		cp.Mappings[i] = m.Clone()
	}
	return &cp
}

// Get returns a deep copy of the current configuration.
func Get() *Config {
	mu.RLock()
	defer mu.RUnlock()
	return cfg.Clone()
}

// Update applies fn to the configuration under the write lock and returns a
// copy of the result. fn must not call other functions of this package.
func Update(fn func(c *Config) error) (*Config, error) {
	mu.Lock()
	defer mu.Unlock()

	work := cfg.Clone()
	if err := fn(work); err != nil {
		return nil, err
	}
	normalize(work)
	if err := validate(work); err != nil {
		return nil, err
	}
	cfg = work
	return cfg.Clone(), nil
}

// Save writes the current configuration to the file atomically. A token that
// came from the environment is not persisted; the file keeps its own value.
func Save() error {
	mu.RLock()
	toSave := cfg
	if envAPIToken != "" && cfg.Server.APIToken == envAPIToken {
		toSave = cfg.Clone()
		toSave.Server.APIToken = fileAPIToken
	}
	data, err := yaml.Marshal(toSave)
	path := pathLocked()
	mu.RUnlock()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.yaml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// UpdateShelly replaces the Shelly configuration
func UpdateShelly(shelly ShellyConfig) error {
	_, err := Update(func(c *Config) error {
		c.Shelly = shelly
		return nil
	})
	return err
}

// UpdateMappings replaces the mappings configuration
func UpdateMappings(mappings []models.Mapping) error {
	_, err := Update(func(c *Config) error {
		c.Mappings = mappings
		return nil
	})
	return err
}

// GetMappings returns a copy of current mappings
func GetMappings() []models.Mapping {
	mu.RLock()
	defer mu.RUnlock()

	result := make([]models.Mapping, len(cfg.Mappings))
	for i, m := range cfg.Mappings {
		result[i] = m.Clone()
	}
	return result
}

// Masked returns a copy of the configuration with all secrets replaced by
// MaskedSecret, suitable for returning to API clients.
func (c *Config) Masked() *Config {
	cp := c.Clone()
	mask := func(s *string) {
		if *s != "" {
			*s = MaskedSecret
		}
	}
	mask(&cp.MQTT.Password)
	mask(&cp.Server.APIToken)
	mask(&cp.Server.WebhookToken)
	for i := range cp.Loxone.Miniservers {
		mask(&cp.Loxone.Miniservers[i].HTTPPassword)
	}
	for i := range cp.Shelly.Devices {
		mask(&cp.Shelly.Devices[i].Password)
	}
	return cp
}

// RestoreSecrets replaces MaskedSecret placeholders in incoming with the
// values currently stored in current. Devices and miniservers are matched by ID.
func RestoreSecrets(incoming, current *Config) {
	keep := func(in *string, cur string) {
		if *in == MaskedSecret {
			*in = cur
		}
	}
	keep(&incoming.MQTT.Password, current.MQTT.Password)
	keep(&incoming.Server.APIToken, current.Server.APIToken)
	keep(&incoming.Server.WebhookToken, current.Server.WebhookToken)

	msByID := make(map[string]MiniserverConfig, len(current.Loxone.Miniservers))
	for _, m := range current.Loxone.Miniservers {
		msByID[m.ID] = m
	}
	for i := range incoming.Loxone.Miniservers {
		if cur, ok := msByID[incoming.Loxone.Miniservers[i].ID]; ok {
			keep(&incoming.Loxone.Miniservers[i].HTTPPassword, cur.HTTPPassword)
		} else if incoming.Loxone.Miniservers[i].HTTPPassword == MaskedSecret {
			incoming.Loxone.Miniservers[i].HTTPPassword = ""
		}
	}

	devByID := make(map[string]ShellyDeviceConfig, len(current.Shelly.Devices))
	for _, d := range current.Shelly.Devices {
		devByID[d.ID] = d
	}
	for i := range incoming.Shelly.Devices {
		if cur, ok := devByID[incoming.Shelly.Devices[i].ID]; ok {
			keep(&incoming.Shelly.Devices[i].Password, cur.Password)
		} else if incoming.Shelly.Devices[i].Password == MaskedSecret {
			incoming.Shelly.Devices[i].Password = ""
		}
	}
}

// GetServer returns a copy of the server section.
func GetServer() ServerConfig {
	mu.RLock()
	defer mu.RUnlock()
	s := cfg.Server
	s.CORSOrigins = append([]string(nil), cfg.Server.CORSOrigins...)
	return s
}
