package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/config"
	"github.com/sbeyeler/loxone2shelly/internal/logging"
	"github.com/sbeyeler/loxone2shelly/internal/models"
	"github.com/sbeyeler/loxone2shelly/internal/shelly"
)

const (
	maxJSONBody = 1 << 20 // 1 MiB
	maxLogLimit = 1000
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// Handlers contains all HTTP handler methods
type Handlers struct {
	deps  Deps
	wsHub *WebSocketHub
}

// jsonResponse writes a JSON response
func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Debug().Err(err).Msg("Failed to write JSON response")
		}
	}
}

// jsonError writes a JSON error response
func jsonError(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}

// persistConfig saves the configuration and reports a failure to the client.
// The change has already been applied in memory at this point, so the client
// is told that only the persistence step failed.
func persistConfig(w http.ResponseWriter, what string) bool {
	if err := config.Save(); err != nil {
		log.Error().Err(err).Msgf("Failed to save config after %s", what)
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("%s applied, but the configuration could not be saved: %v", what, err))
		return false
	}
	return true
}

// decodeJSON decodes a request body with a size limit. Unknown fields are
// ignored so older exports and Loxone-generated bodies keep working.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func validID(id string) bool {
	return idPattern.MatchString(id)
}

// validHostPort accepts an IPv4/IPv6 address or hostname with optional port.
func validHostPort(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	host := s
	if h, _, err := net.SplitHostPort(s); err == nil {
		host = h
	}
	if net.ParseIP(host) != nil {
		return true
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, r := range label {
			if !(r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
				return false
			}
		}
	}
	return true
}

func toSpecs(devices []config.ShellyDeviceConfig) []shelly.DeviceSpec {
	specs := make([]shelly.DeviceSpec, 0, len(devices))
	for _, d := range devices {
		specs = append(specs, shelly.DeviceSpec{
			ID: d.ID, Name: d.Name, IP: d.IP, Generation: d.Generation,
			Username: d.Username, Password: d.Password,
		})
	}
	return specs
}

// applyConfig pushes a configuration into all runtime components. When
// previous is non-nil, MQTT is only restarted if its settings changed.
func (h *Handlers) applyConfig(previous, cfg *config.Config) {
	h.deps.Shelly.SyncDevices(toSpecs(cfg.Shelly.Devices))
	h.deps.Shelly.SetPollInterval(cfg.Shelly.PollInterval)
	h.deps.Mappings.Load(cfg.Mappings)
	h.deps.UDP.Configure(cfg.Loxone.Miniservers)
	h.deps.HTTP.Configure(cfg.Loxone.Miniservers)

	mqttChanged := previous == nil || previous.MQTT != cfg.MQTT
	miniserversChanged := previous == nil || !equalMiniservers(previous.Loxone.Miniservers, cfg.Loxone.Miniservers)
	if h.deps.MQTT != nil && (mqttChanged || miniserversChanged) {
		h.deps.MQTT.Configure(cfg.MQTT, cfg.Loxone.Miniservers)
	}
	if h.deps.MQTTSub != nil {
		h.deps.MQTTSub.SetDevices(cfg.Shelly.Devices)
		if mqttChanged {
			if err := h.deps.MQTTSub.Restart(cfg.MQTT); err != nil {
				log.Error().Err(err).Msg("Failed to restart MQTT subscriber")
			}
		}
	}
	if h.deps.SetLogLevel != nil && (previous == nil || previous.Logging.Level != cfg.Logging.Level) {
		h.deps.SetLogLevel(cfg.Logging.Level)
	}
}

func equalMiniservers(a, b []config.MiniserverConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Health returns the health status
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	resp := map[string]interface{}{
		"status":                 "ok",
		"version":                config.GetVersion(),
		"timestamp":              time.Now().UTC().Format(time.RFC3339),
		"shelly_configured":      h.deps.Shelly.IsConfigured(),
		"auth_required":          config.GetServer().APIToken != "",
		"webhook_token_required": config.GetServer().WebhookToken != "",
		// The UI needs the port to build device-reachable URLs when it is
		// served through a proxy (e.g. Home Assistant ingress).
		"port": config.GetServer().Port,
	}
	if h.deps.MQTTSub != nil {
		resp["mqtt_connected"] = h.deps.MQTTSub.IsConnected()
	}
	jsonResponse(w, http.StatusOK, resp)
}

// GetDevices returns all configured Shelly devices with their last known state.
// ?refresh=1 forces a live query of all devices.
func (h *Handlers) GetDevices(w http.ResponseWriter, r *http.Request) {
	var devices []*models.Device
	if r.URL.Query().Get("refresh") == "1" {
		devices = h.deps.Shelly.RefreshAll()
	} else {
		devices = h.deps.Shelly.GetDevices()
	}
	jsonResponse(w, http.StatusOK, devices)
}

// GetDevice returns a single Shelly device by ID
func (h *Handlers) GetDevice(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var device *models.Device
	var err error
	if r.URL.Query().Get("refresh") == "1" {
		device, err = h.deps.Shelly.GetDeviceLive(id)
	} else {
		device, err = h.deps.Shelly.GetDevice(id)
	}
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, device)
}

// SetDevice sends a command to a Shelly device
func (h *Handlers) SetDevice(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var cmd models.DeviceCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.deps.Shelly.SetDeviceState(id, cmd); err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		} else if strings.Contains(err.Error(), "must be") || strings.Contains(err.Error(), "no actionable") {
			status = http.StatusBadRequest
		}
		jsonError(w, status, err.Error())
		return
	}

	// Read back the state so UI and Loxone see the result immediately.
	device, err := h.deps.Shelly.GetDeviceLive(id)
	if err != nil {
		jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	jsonResponse(w, http.StatusOK, device)
}

// GetInputs returns all inputs from all configured Shelly devices
func (h *Handlers) GetInputs(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, h.deps.Shelly.GetInputs())
}

// DiscoverShelly runs an mDNS discovery for Shelly devices
func (h *Handlers) DiscoverShelly(w http.ResponseWriter, r *http.Request) {
	devices, err := shelly.DiscoverDevices(r.Context(), 4*time.Second)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "discovery failed: "+err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, devices)
}

// ShellyDeviceRequest is the request body for adding or updating a Shelly device.
// Pointer fields distinguish "not provided" from "empty".
type ShellyDeviceRequest struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	IP         string  `json:"ip"`
	Generation int     `json:"generation"`
	MQTTID     *string `json:"mqtt_id"`
	Username   *string `json:"username"`
	Password   *string `json:"password"`
}

func deviceResponse(d config.ShellyDeviceConfig) map[string]interface{} {
	resp := map[string]interface{}{
		"id":         d.ID,
		"name":       d.Name,
		"ip":         d.IP,
		"generation": d.Generation,
		"mqtt_id":    d.MQTTID,
		"username":   d.Username,
		"password":   "",
	}
	if d.Password != "" {
		resp["password"] = config.MaskedSecret
	}
	return resp
}

// AddShelly adds a new Shelly device to the configuration
func (h *Handlers) AddShelly(w http.ResponseWriter, r *http.Request) {
	var req ShellyDeviceRequest
	if err := decodeJSON(w, r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	if !validHostPort(req.IP) {
		jsonError(w, http.StatusBadRequest, "ip must be a valid IP address or hostname")
		return
	}
	if req.ID == "" {
		req.ID = uuid.New().String()[:8]
	}
	if !validID(req.ID) {
		jsonError(w, http.StatusBadRequest, "id may only contain letters, digits, '_', '-' and '.' (max 64 chars)")
		return
	}
	if req.Name == "" {
		req.Name = "Shelly " + req.ID
	}
	if req.Generation == 0 {
		req.Generation = 2
	}
	if req.Generation < 1 || req.Generation > 4 {
		jsonError(w, http.StatusBadRequest, "generation must be between 1 and 4")
		return
	}

	dev := config.ShellyDeviceConfig{
		ID:         req.ID,
		Name:       req.Name,
		IP:         req.IP,
		Generation: req.Generation,
	}
	if req.MQTTID != nil {
		dev.MQTTID = *req.MQTTID
	}
	if req.Username != nil {
		dev.Username = *req.Username
	}
	if req.Password != nil && *req.Password != config.MaskedSecret {
		dev.Password = *req.Password
	}

	cfg, err := config.Update(func(c *config.Config) error {
		for _, d := range c.Shelly.Devices {
			if d.ID == dev.ID {
				return fmt.Errorf("device id %q already exists", dev.ID)
			}
		}
		c.Shelly.Devices = append(c.Shelly.Devices, dev)
		return nil
	})
	if err != nil {
		jsonError(w, http.StatusConflict, err.Error())
		return
	}

	h.deps.Shelly.AddDevice(toSpecs([]config.ShellyDeviceConfig{dev})[0])
	if h.deps.MQTTSub != nil {
		h.deps.MQTTSub.SetDevices(cfg.Shelly.Devices)
	}
	if !persistConfig(w, "adding device") {
		return
	}

	log.Info().Str("id", dev.ID).Str("ip", dev.IP).Msg("Shelly device added via API")
	jsonResponse(w, http.StatusCreated, deviceResponse(dev))
}

// UpdateShelly updates an existing Shelly device in the configuration
func (h *Handlers) UpdateShelly(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var req ShellyDeviceRequest
	if err := decodeJSON(w, r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.IP != "" && !validHostPort(req.IP) {
		jsonError(w, http.StatusBadRequest, "ip must be a valid IP address or hostname")
		return
	}
	if req.Generation != 0 && (req.Generation < 1 || req.Generation > 4) {
		jsonError(w, http.StatusBadRequest, "generation must be between 1 and 4")
		return
	}

	var updated config.ShellyDeviceConfig
	cfg, err := config.Update(func(c *config.Config) error {
		for i := range c.Shelly.Devices {
			d := &c.Shelly.Devices[i]
			if d.ID != id {
				continue
			}
			if req.Name != "" {
				d.Name = req.Name
			}
			if req.IP != "" {
				d.IP = req.IP
			}
			if req.Generation > 0 {
				d.Generation = req.Generation
			}
			if req.MQTTID != nil {
				d.MQTTID = *req.MQTTID
			}
			if req.Username != nil {
				d.Username = *req.Username
			}
			if req.Password != nil && *req.Password != config.MaskedSecret {
				d.Password = *req.Password
			}
			updated = *d
			return nil
		}
		return errNotFound
	})
	if err == errNotFound {
		jsonError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.deps.Shelly.UpdateDevice(toSpecs([]config.ShellyDeviceConfig{updated})[0])
	if h.deps.MQTTSub != nil {
		h.deps.MQTTSub.SetDevices(cfg.Shelly.Devices)
	}
	if !persistConfig(w, "updating device") {
		return
	}

	log.Info().Str("id", id).Msg("Shelly device updated via API")
	jsonResponse(w, http.StatusOK, deviceResponse(updated))
}

var errNotFound = fmt.Errorf("not found")

// RemoveShelly removes a Shelly device from the configuration
func (h *Handlers) RemoveShelly(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	cfg, err := config.Update(func(c *config.Config) error {
		newDevices := make([]config.ShellyDeviceConfig, 0, len(c.Shelly.Devices))
		found := false
		for _, d := range c.Shelly.Devices {
			if d.ID == id {
				found = true
				continue
			}
			newDevices = append(newDevices, d)
		}
		if !found {
			return errNotFound
		}
		c.Shelly.Devices = newDevices
		return nil
	})
	if err == errNotFound {
		jsonError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.deps.Shelly.RemoveDevice(id)
	if h.deps.MQTTSub != nil {
		h.deps.MQTTSub.SetDevices(cfg.Shelly.Devices)
	}
	if !persistConfig(w, "removing device") {
		return
	}

	log.Info().Str("id", id).Msg("Shelly device removed via API")
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// TestShellyConnection tests the connection to a Shelly device
func (h *Handlers) TestShellyConnection(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	device, err := h.deps.Shelly.GetDeviceLive(id)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}

	if !device.Online {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Device %s (%s) is not reachable", device.Name, device.IP),
		})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Connection successful",
		"device":  device,
	})
}

// --- Mapping handlers ---

func validateMapping(m *models.Mapping) error {
	if m.LoxoneID == "" || m.ShellyID == "" {
		return fmt.Errorf("loxone_id and shelly_id are required")
	}
	if len(m.LoxoneID) > 64 || strings.ContainsAny(m.LoxoneID, " /:\t\n") {
		return fmt.Errorf("loxone_id must not contain spaces, '/' or ':' (max 64 chars)")
	}
	if m.Channel != nil && (*m.Channel < 0 || *m.Channel > 15) {
		return fmt.Errorf("channel must be between 0 and 15")
	}
	return nil
}

// GetMappings returns all mappings
func (h *Handlers) GetMappings(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, config.GetMappings())
}

// CreateMapping creates a new mapping
func (h *Handlers) CreateMapping(w http.ResponseWriter, r *http.Request) {
	var mapping models.Mapping
	if err := decodeJSON(w, r, &mapping); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if mapping.ID == "" {
		mapping.ID = uuid.New().String()
	}
	if err := validateMapping(&mapping); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	cfg, err := config.Update(func(c *config.Config) error {
		for _, m := range c.Mappings {
			if m.ID == mapping.ID {
				return fmt.Errorf("mapping id %q already exists", mapping.ID)
			}
			if m.Enabled && mapping.Enabled && m.LoxoneID == mapping.LoxoneID {
				return fmt.Errorf("loxone_id %q is already used by mapping %q", mapping.LoxoneID, m.Name)
			}
		}
		c.Mappings = append(c.Mappings, mapping)
		return nil
	})
	if err != nil {
		jsonError(w, http.StatusConflict, err.Error())
		return
	}

	h.deps.Mappings.Load(cfg.Mappings)
	if !persistConfig(w, "creating mapping") {
		return
	}

	jsonResponse(w, http.StatusCreated, mapping)
}

// UpdateMapping updates an existing mapping
func (h *Handlers) UpdateMapping(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updated models.Mapping
	if err := decodeJSON(w, r, &updated); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated.ID = id
	if err := validateMapping(&updated); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	cfg, err := config.Update(func(c *config.Config) error {
		idx := -1
		for i, m := range c.Mappings {
			if m.ID == id {
				idx = i
			} else if m.Enabled && updated.Enabled && m.LoxoneID == updated.LoxoneID {
				return fmt.Errorf("loxone_id %q is already used by mapping %q", updated.LoxoneID, m.Name)
			}
		}
		if idx < 0 {
			return errNotFound
		}
		c.Mappings[idx] = updated
		return nil
	})
	if err == errNotFound {
		jsonError(w, http.StatusNotFound, "mapping not found")
		return
	}
	if err != nil {
		jsonError(w, http.StatusConflict, err.Error())
		return
	}

	h.deps.Mappings.Load(cfg.Mappings)
	if !persistConfig(w, "updating mapping") {
		return
	}

	jsonResponse(w, http.StatusOK, updated)
}

// DeleteMapping deletes a mapping
func (h *Handlers) DeleteMapping(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	cfg, err := config.Update(func(c *config.Config) error {
		newMappings := make([]models.Mapping, 0, len(c.Mappings))
		found := false
		for _, m := range c.Mappings {
			if m.ID == id {
				found = true
				continue
			}
			newMappings = append(newMappings, m)
		}
		if !found {
			return errNotFound
		}
		c.Mappings = newMappings
		return nil
	})
	if err == errNotFound {
		jsonError(w, http.StatusNotFound, "mapping not found")
		return
	}
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.deps.Mappings.Load(cfg.Mappings)
	if !persistConfig(w, "deleting mapping") {
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ExportMappings exports all mappings as JSON
func (h *Handlers) ExportMappings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Disposition", "attachment; filename=mappings.json")
	jsonResponse(w, http.StatusOK, config.GetMappings())
}

// ImportMappings imports mappings from a JSON body. Existing IDs are replaced,
// new ones appended.
func (h *Handlers) ImportMappings(w http.ResponseWriter, r *http.Request) {
	var imported []models.Mapping
	if err := decodeJSON(w, r, &imported); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	for i := range imported {
		if imported[i].ID == "" {
			imported[i].ID = uuid.New().String()
		}
		if err := validateMapping(&imported[i]); err != nil {
			jsonError(w, http.StatusBadRequest, fmt.Sprintf("mapping %d: %v", i+1, err))
			return
		}
	}

	added, replaced := 0, 0
	cfg, err := config.Update(func(c *config.Config) error {
		byID := make(map[string]int, len(c.Mappings))
		for i, m := range c.Mappings {
			byID[m.ID] = i
		}
		for _, m := range imported {
			if idx, ok := byID[m.ID]; ok {
				c.Mappings[idx] = m
				replaced++
			} else {
				byID[m.ID] = len(c.Mappings)
				c.Mappings = append(c.Mappings, m)
				added++
			}
		}
		// Same rule as create/update: a loxone_id may only be active once.
		seen := make(map[string]string, len(c.Mappings))
		for _, m := range c.Mappings {
			if !m.Enabled {
				continue
			}
			if other, dup := seen[m.LoxoneID]; dup {
				return fmt.Errorf("loxone_id %q is used by both %q and %q", m.LoxoneID, other, m.Name)
			}
			seen[m.LoxoneID] = m.Name
		}
		return nil
	})
	if err != nil {
		jsonError(w, http.StatusConflict, err.Error())
		return
	}

	h.deps.Mappings.Load(cfg.Mappings)
	if !persistConfig(w, "importing mappings") {
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"imported": len(imported),
		"added":    added,
		"replaced": replaced,
		"total":    len(cfg.Mappings),
	})
}

// --- Config handlers ---

// ConfigResponse is the response for the config endpoint. Secrets are masked.
type ConfigResponse struct {
	Server  config.ServerConfig  `json:"server"`
	Shelly  config.ShellyConfig  `json:"shelly"`
	Loxone  config.LoxoneConfig  `json:"loxone"`
	MQTT    config.MQTTConfig    `json:"mqtt"`
	Logging config.LoggingConfig `json:"logging"`
}

// GetConfig returns the current configuration with secrets masked
func (h *Handlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get().Masked()
	jsonResponse(w, http.StatusOK, ConfigResponse{
		Server:  cfg.Server,
		Shelly:  cfg.Shelly,
		Loxone:  cfg.Loxone,
		MQTT:    cfg.MQTT,
		Logging: cfg.Logging,
	})
}

// UpdateConfig replaces the configuration (except mappings) and applies it
// to all runtime components. Masked secrets keep their stored value.
func (h *Handlers) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var update ConfigResponse
	if err := decodeJSON(w, r, &update); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	for _, d := range update.Shelly.Devices {
		if !validID(d.ID) {
			jsonError(w, http.StatusBadRequest, fmt.Sprintf("invalid device id %q", d.ID))
			return
		}
		if !validHostPort(d.IP) {
			jsonError(w, http.StatusBadRequest, fmt.Sprintf("device %q: invalid ip", d.ID))
			return
		}
	}
	for _, m := range update.Loxone.Miniservers {
		if !validID(m.ID) {
			jsonError(w, http.StatusBadRequest, fmt.Sprintf("invalid miniserver id %q", m.ID))
			return
		}
	}

	previous := config.Get()
	cfg, err := config.Update(func(c *config.Config) error {
		incoming := &config.Config{
			Server:  update.Server,
			Shelly:  update.Shelly,
			Loxone:  update.Loxone,
			MQTT:    update.MQTT,
			Logging: update.Logging,
		}
		config.RestoreSecrets(incoming, c)
		// The listening address is persisted but only applied after a restart;
		// omitted values keep the current ones.
		if incoming.Server.Port == 0 {
			incoming.Server.Port = c.Server.Port
		}
		if incoming.Server.Host == "" {
			incoming.Server.Host = c.Server.Host
		}
		c.Server = incoming.Server
		c.Shelly = incoming.Shelly
		c.Loxone = incoming.Loxone
		c.MQTT = incoming.MQTT
		c.Logging = incoming.Logging
		return nil
	})
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.applyConfig(previous, cfg)

	if err := config.Save(); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	log.Info().Msg("Configuration updated via API")
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":           "ok",
		"restart_required": cfg.Server.Port != previous.Server.Port || cfg.Server.Host != previous.Server.Host,
	})
}

// TestUDP sends a test UDP message
func (h *Handlers) TestUDP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MiniserverID string `json:"miniserver_id"`
		LoxoneID     string `json:"loxone_id"`
		Property     string `json:"property"`
		Value        string `json:"value"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.LoxoneID == "" || req.Property == "" {
		jsonError(w, http.StatusBadRequest, "loxone_id and property are required")
		return
	}

	if !validateUDPValue(req.Property, req.Value) {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("invalid value '%s' for property '%s'", req.Value, req.Property))
		return
	}

	if !h.deps.UDP.IsEnabled() {
		jsonError(w, http.StatusBadRequest, "no UDP targets configured")
		return
	}

	if req.MiniserverID != "" {
		if !h.deps.UDP.IsEnabledFor(req.MiniserverID) {
			jsonError(w, http.StatusBadRequest, "UDP is not enabled for this miniserver")
			return
		}
		h.deps.UDP.Send(req.MiniserverID, req.LoxoneID, req.Property, req.Value)
	} else {
		h.deps.UDP.SendToAll(req.LoxoneID, req.Property, req.Value)
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Sent %s/%s:%s", req.LoxoneID, req.Property, req.Value),
	})
}

// validateUDPValue validates a value for a given Shelly property
func validateUDPValue(property, value string) bool {
	switch property {
	case "on":
		return value == "0" || value == "1"
	case "bri", "pos":
		v, err := strconv.ParseFloat(value, 64)
		return err == nil && v >= 0 && v <= 100
	case "power":
		_, err := strconv.ParseFloat(value, 64)
		return err == nil
	default:
		return len(value) <= 64
	}
}

// GetLogs returns log entries
func (h *Handlers) GetLogs(w http.ResponseWriter, r *http.Request) {
	level := r.URL.Query().Get("level")
	search := r.URL.Query().Get("search")
	limitStr := r.URL.Query().Get("limit")

	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > maxLogLimit {
		limit = maxLogLimit
	}

	jsonResponse(w, http.StatusOK, logging.Buffer.GetEntries(level, search, limit))
}
