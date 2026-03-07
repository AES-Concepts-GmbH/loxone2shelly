package config

import (
	"os"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/models"
	"gopkg.in/yaml.v3"
)

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
}

// ShellyDeviceConfig holds settings for a single Shelly device
type ShellyDeviceConfig struct {
	ID         string `yaml:"id" json:"id"`
	Name       string `yaml:"name" json:"name"`
	IP         string `yaml:"ip" json:"ip"`
	Generation int    `yaml:"generation" json:"generation"` // 1 or 2
	MQTTID     string `yaml:"mqtt_id" json:"mqtt_id"`       // MQTT device ID (e.g. "shellyplus1-aabbccddee"); defaults to ID
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
	cfg        *Config
	cfgOnce    sync.Once
	cfgPath    string
	mu         sync.RWMutex
	appVersion string
)

// SetVersion sets the application version for use in backups
func SetVersion(v string) {
	appVersion = v
}

// GetVersion returns the application version
func GetVersion() string {
	if appVersion == "" {
		return "dev"
	}
	return appVersion
}

// GetPath returns the current config file path
func GetPath() string {
	mu.RLock()
	defer mu.RUnlock()
	if cfgPath == "" {
		return "configs/config.yaml"
	}
	return cfgPath
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

// Load reads the configuration from a YAML file
func Load(path string) (*Config, error) {
	cfgPath = path

	cfgOnce.Do(func() {
		cfg = DefaultConfig()
	})

	mu.Lock()
	defer mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Info().Str("path", path).Msg("Config file not found, using defaults")
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Ensure devices is never nil
	if cfg.Shelly.Devices == nil {
		cfg.Shelly.Devices = []ShellyDeviceConfig{}
	}

	// Ensure default poll interval
	if cfg.Shelly.PollInterval <= 0 {
		cfg.Shelly.PollInterval = 10
	}

	// Ensure miniservers is never nil
	if cfg.Loxone.Miniservers == nil {
		cfg.Loxone.Miniservers = []MiniserverConfig{}
	}

	// Migrate mappings: assign first miniserver to mappings without miniserver_id
	if len(cfg.Loxone.Miniservers) > 0 {
		firstID := cfg.Loxone.Miniservers[0].ID
		for i := range cfg.Mappings {
			if cfg.Mappings[i].MiniserverID == "" {
				cfg.Mappings[i].MiniserverID = firstID
			}
		}
	}

	log.Info().Str("path", path).Msg("Configuration loaded")
	return cfg, nil
}

// Get returns the current configuration
func Get() *Config {
	mu.RLock()
	defer mu.RUnlock()
	return cfg
}

// Save writes the current configuration to the file
func Save() error {
	mu.RLock()
	defer mu.RUnlock()

	if cfgPath == "" {
		cfgPath = "config.yaml"
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(cfgPath, data, 0644)
}

// UpdateShelly updates the Shelly configuration
func UpdateShelly(shelly ShellyConfig) {
	mu.Lock()
	defer mu.Unlock()

	cfg.Shelly = shelly
}

// UpdateMappings updates the mappings configuration
func UpdateMappings(mappings []models.Mapping) {
	mu.Lock()
	defer mu.Unlock()

	cfg.Mappings = mappings
}

// GetMappings returns a copy of current mappings
func GetMappings() []models.Mapping {
	mu.RLock()
	defer mu.RUnlock()

	result := make([]models.Mapping, len(cfg.Mappings))
	copy(result, cfg.Mappings)
	return result
}

// RestoreFromBackup restores all configuration from a backup
func RestoreFromBackup(server ServerConfig, shelly ShellyConfig, loxone LoxoneConfig, logging LoggingConfig, mappings []models.Mapping) {
	mu.Lock()
	defer mu.Unlock()

	cfg.Server = server
	cfg.Shelly = shelly
	cfg.Loxone = loxone
	cfg.Logging = logging
	cfg.Mappings = mappings
}
