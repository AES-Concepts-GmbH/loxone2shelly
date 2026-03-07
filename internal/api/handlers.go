package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/config"
	"github.com/sbeyeler/loxone2shelly/internal/logging"
	"github.com/sbeyeler/loxone2shelly/internal/loxone"
	"github.com/sbeyeler/loxone2shelly/internal/models"
	"github.com/sbeyeler/loxone2shelly/internal/shelly"
)

// Handlers contains all HTTP handler methods
type Handlers struct {
	shellyClient   *shelly.Client
	mappingManager *loxone.MappingManager
	udpSender      *loxone.UDPSender
	httpSender     *loxone.HTTPSender
}

// jsonResponse writes a JSON response
func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// jsonError writes a JSON error response
func jsonError(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}

// Health returns the health status
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":            "ok",
		"timestamp":         time.Now().UTC().Format(time.RFC3339),
		"shelly_configured": h.shellyClient.IsConfigured(),
	})
}

// GetDevices returns all configured Shelly devices with their current state
func (h *Handlers) GetDevices(w http.ResponseWriter, r *http.Request) {
	devices := h.shellyClient.GetDevices()
	jsonResponse(w, http.StatusOK, devices)
}

// GetDevice returns a single Shelly device by ID
func (h *Handlers) GetDevice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	device, err := h.shellyClient.GetDevice(id)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, device)
}

// SetDevice sends a command to a Shelly device
func (h *Handlers) SetDevice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var cmd models.DeviceCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if err := h.shellyClient.SetDeviceState(id, cmd); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Read back state
	device, err := h.shellyClient.GetDevice(id)
	if err != nil {
		jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	jsonResponse(w, http.StatusOK, device)
}

// GetInputs returns all inputs from all configured Shelly devices
func (h *Handlers) GetInputs(w http.ResponseWriter, r *http.Request) {
	inputs := h.shellyClient.GetInputs()
	jsonResponse(w, http.StatusOK, inputs)
}

// DiscoverShelly initiates Shelly device discovery
func (h *Handlers) DiscoverShelly(w http.ResponseWriter, r *http.Request) {
	devices, err := shelly.DiscoverDevices(5 * time.Second)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "discovery failed: "+err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, devices)
}

// AddShellyRequest is the request body for adding a new Shelly device
type AddShellyRequest struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IP         string `json:"ip"`
	Generation int    `json:"generation"`
}

// AddShelly adds a new Shelly device to the configuration
func (h *Handlers) AddShelly(w http.ResponseWriter, r *http.Request) {
	var req AddShellyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.IP == "" {
		jsonError(w, http.StatusBadRequest, "ip is required")
		return
	}
	if req.ID == "" {
		req.ID = uuid.New().String()[:8]
	}
	if req.Name == "" {
		req.Name = "Shelly " + req.ID
	}
	if req.Generation == 0 {
		req.Generation = 2
	}

	// Add to Shelly client
	h.shellyClient.AddDevice(req.ID, req.Name, req.IP, req.Generation)

	// Add to config and save
	cfg := config.Get()
	cfg.Shelly.Devices = append(cfg.Shelly.Devices, config.ShellyDeviceConfig{
		ID:         req.ID,
		Name:       req.Name,
		IP:         req.IP,
		Generation: req.Generation,
	})
	config.UpdateShelly(cfg.Shelly)
	if err := config.Save(); err != nil {
		log.Error().Err(err).Msg("Failed to save config after adding device")
	}

	log.Info().Str("id", req.ID).Str("ip", req.IP).Msg("Shelly device added via API")

	jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"id":         req.ID,
		"name":       req.Name,
		"ip":         req.IP,
		"generation": req.Generation,
	})
}

// RemoveShelly removes a Shelly device from the configuration
func (h *Handlers) RemoveShelly(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	h.shellyClient.RemoveDevice(id)

	// Remove from config
	cfg := config.Get()
	newDevices := make([]config.ShellyDeviceConfig, 0, len(cfg.Shelly.Devices))
	for _, d := range cfg.Shelly.Devices {
		if d.ID != id {
			newDevices = append(newDevices, d)
		}
	}
	cfg.Shelly.Devices = newDevices
	config.UpdateShelly(cfg.Shelly)
	if err := config.Save(); err != nil {
		log.Error().Err(err).Msg("Failed to save config after removing device")
	}

	log.Info().Str("id", id).Msg("Shelly device removed via API")

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// TestShellyConnection tests the connection to a Shelly device
func (h *Handlers) TestShellyConnection(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	device, err := h.shellyClient.GetDevice(id)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}

	// Try to reach the device
	client := &http.Client{Timeout: 5 * time.Second}

	var testURL string
	if device.Generation >= 2 {
		testURL = fmt.Sprintf("http://%s/rpc/Shelly.GetDeviceInfo", device.IP)
	} else {
		testURL = fmt.Sprintf("http://%s/shelly", device.IP)
	}

	resp, err := client.Get(testURL)
	if err != nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"id":        id,
			"reachable": false,
			"error":     err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var info map[string]interface{}
	json.Unmarshal(body, &info)

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"id":        id,
		"reachable": true,
		"status":    resp.StatusCode,
		"info":      info,
	})
}

// --- Mapping handlers ---

// GetMappings returns all mappings
func (h *Handlers) GetMappings(w http.ResponseWriter, r *http.Request) {
	mappings := config.GetMappings()
	jsonResponse(w, http.StatusOK, mappings)
}

// CreateMapping creates a new mapping
func (h *Handlers) CreateMapping(w http.ResponseWriter, r *http.Request) {
	var mapping models.Mapping
	if err := json.NewDecoder(r.Body).Decode(&mapping); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if mapping.ID == "" {
		mapping.ID = uuid.New().String()
	}
	if mapping.LoxoneID == "" || mapping.ShellyID == "" {
		jsonError(w, http.StatusBadRequest, "loxone_id and shelly_id are required")
		return
	}

	// Add to config
	mappings := config.GetMappings()
	mappings = append(mappings, mapping)
	config.UpdateMappings(mappings)

	// Update mapping manager
	h.mappingManager.Load(mappings)

	if err := config.Save(); err != nil {
		log.Error().Err(err).Msg("Failed to save config after creating mapping")
	}

	jsonResponse(w, http.StatusCreated, mapping)
}

// UpdateMapping updates an existing mapping
func (h *Handlers) UpdateMapping(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var updated models.Mapping
	if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	updated.ID = id

	mappings := config.GetMappings()
	found := false
	for i, m := range mappings {
		if m.ID == id {
			mappings[i] = updated
			found = true
			break
		}
	}

	if !found {
		jsonError(w, http.StatusNotFound, "mapping not found")
		return
	}

	config.UpdateMappings(mappings)
	h.mappingManager.Load(mappings)

	if err := config.Save(); err != nil {
		log.Error().Err(err).Msg("Failed to save config after updating mapping")
	}

	jsonResponse(w, http.StatusOK, updated)
}

// DeleteMapping deletes a mapping
func (h *Handlers) DeleteMapping(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	mappings := config.GetMappings()
	newMappings := make([]models.Mapping, 0, len(mappings))
	found := false
	for _, m := range mappings {
		if m.ID == id {
			found = true
			continue
		}
		newMappings = append(newMappings, m)
	}

	if !found {
		jsonError(w, http.StatusNotFound, "mapping not found")
		return
	}

	config.UpdateMappings(newMappings)
	h.mappingManager.Remove(id)

	if err := config.Save(); err != nil {
		log.Error().Err(err).Msg("Failed to save config after deleting mapping")
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ExportMappings exports all mappings as JSON
func (h *Handlers) ExportMappings(w http.ResponseWriter, r *http.Request) {
	mappings := config.GetMappings()
	w.Header().Set("Content-Disposition", "attachment; filename=mappings.json")
	jsonResponse(w, http.StatusOK, mappings)
}

// ImportMappings imports mappings from a JSON body
func (h *Handlers) ImportMappings(w http.ResponseWriter, r *http.Request) {
	var imported []models.Mapping
	if err := json.NewDecoder(r.Body).Decode(&imported); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Assign new IDs to avoid conflicts
	for i := range imported {
		if imported[i].ID == "" {
			imported[i].ID = uuid.New().String()
		}
	}

	// Merge with existing
	existing := config.GetMappings()
	existing = append(existing, imported...)
	config.UpdateMappings(existing)
	h.mappingManager.Load(existing)

	if err := config.Save(); err != nil {
		log.Error().Err(err).Msg("Failed to save config after importing mappings")
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"imported": len(imported),
		"total":    len(existing),
	})
}

// --- Config handlers ---

// ConfigResponse is the response for the config endpoint
type ConfigResponse struct {
	Server  config.ServerConfig  `json:"server"`
	Shelly  config.ShellyConfig  `json:"shelly"`
	Loxone  config.LoxoneConfig  `json:"loxone"`
	Logging config.LoggingConfig `json:"logging"`
}

// GetConfig returns the current configuration
func (h *Handlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	resp := ConfigResponse{
		Server:  cfg.Server,
		Shelly:  cfg.Shelly,
		Loxone:  cfg.Loxone,
		Logging: cfg.Logging,
	}
	jsonResponse(w, http.StatusOK, resp)
}

// UpdateConfig updates the configuration
func (h *Handlers) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var update ConfigResponse
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	cfg := config.Get()
	cfg.Server = update.Server
	cfg.Shelly = update.Shelly
	cfg.Loxone = update.Loxone
	cfg.Logging = update.Logging

	config.UpdateShelly(update.Shelly)

	// Reconfigure UDP and HTTP senders
	h.udpSender.Configure(update.Loxone.Miniservers)
	h.httpSender.Configure(update.Loxone.Miniservers)

	if err := config.Save(); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	log.Info().Msg("Configuration updated via API")
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// TestUDP sends a test UDP message
func (h *Handlers) TestUDP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LoxoneID string `json:"loxone_id"`
		Property string `json:"property"`
		Value    string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
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

	if !h.udpSender.IsEnabled() {
		jsonError(w, http.StatusBadRequest, "no UDP targets configured")
		return
	}

	h.udpSender.SendToAll(req.LoxoneID, req.Property, req.Value)

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// validateUDPValue validates a value for a given Shelly property
func validateUDPValue(property, value string) bool {
	switch property {
	case "on":
		return value == "0" || value == "1"
	case "bri":
		v, err := strconv.ParseFloat(value, 64)
		return err == nil && v >= 0 && v <= 100
	case "pos":
		v, err := strconv.Atoi(value)
		return err == nil && v >= 0 && v <= 100
	case "power":
		_, err := strconv.ParseFloat(value, 64)
		return err == nil
	default:
		return true
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

	entries := logging.Buffer.GetEntries(level, search, limit)
	jsonResponse(w, http.StatusOK, entries)
}
