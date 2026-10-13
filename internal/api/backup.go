package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/config"
	"github.com/sbeyeler/loxone2shelly/internal/models"
)

var backupIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// BackupConfig represents the configuration part of a backup file
type BackupConfig struct {
	Server   config.ServerConfig  `json:"server"`
	Shelly   config.ShellyConfig  `json:"shelly"`
	Loxone   config.LoxoneConfig  `json:"loxone"`
	MQTT     config.MQTTConfig    `json:"mqtt"`
	Logging  config.LoggingConfig `json:"logging"`
	Mappings []models.Mapping     `json:"mappings"`
}

// BackupMeta holds metadata about a backup file (API representation)
type BackupMeta struct {
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	CreatedAt time.Time `json:"created_at"`
	Size      int64     `json:"size"`
	Version   string    `json:"version,omitempty"`
	Remark    string    `json:"remark,omitempty"`
}

// BackupFile wraps metadata and config for a complete backup
type BackupFile struct {
	Meta   BackupFileMeta `json:"meta"`
	Config BackupConfig   `json:"config"`
}

// BackupFileMeta is the metadata stored inside a backup file
type BackupFileMeta struct {
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	Remark    string    `json:"remark,omitempty"`
}

func ensureBackupDir() (string, error) {
	dir := config.BackupDir()
	return dir, os.MkdirAll(dir, 0o750)
}

func backupFilePath(dir, id string) (string, bool) {
	if !backupIDPattern.MatchString(id) {
		return "", false
	}
	return filepath.Join(dir, id+".json"), true
}

func snapshotConfig(cfg *config.Config) BackupConfig {
	return BackupConfig{
		Server:   cfg.Server,
		Shelly:   cfg.Shelly,
		Loxone:   cfg.Loxone,
		MQTT:     cfg.MQTT,
		Logging:  cfg.Logging,
		Mappings: cfg.Mappings,
	}
}

// ListBackups returns all available backups
func (h *Handlers) ListBackups(w http.ResponseWriter, r *http.Request) {
	dir, err := ensureBackupDir()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to access backup directory")
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to read backup directory")
		return
	}

	backups := make([]BackupMeta, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".json")
		meta := BackupMeta{
			ID:        id,
			Filename:  entry.Name(),
			CreatedAt: info.ModTime(),
			Size:      info.Size(),
		}

		if data, err := os.ReadFile(filepath.Join(dir, entry.Name())); err == nil {
			var bf BackupFile
			if json.Unmarshal(data, &bf) == nil {
				meta.Version = bf.Meta.Version
				meta.Remark = bf.Meta.Remark
				if !bf.Meta.CreatedAt.IsZero() {
					meta.CreatedAt = bf.Meta.CreatedAt
				}
			}
		}
		backups = append(backups, meta)
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	jsonResponse(w, http.StatusOK, backups)
}

// CreateBackup creates a new backup of the current configuration
func (h *Handlers) CreateBackup(w http.ResponseWriter, r *http.Request) {
	dir, err := ensureBackupDir()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to create backup directory")
		return
	}

	var req struct {
		Remark string `json:"remark"`
	}
	if r.ContentLength != 0 {
		if err := decodeJSON(w, r, &req); err != nil && err != io.EOF {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if len(req.Remark) > 200 {
		req.Remark = req.Remark[:200]
	}

	now := time.Now()
	id := now.Format("20060102-150405")

	backup := BackupFile{
		Meta: BackupFileMeta{
			Version:   config.GetVersion(),
			CreatedAt: now,
			Remark:    req.Remark,
		},
		Config: snapshotConfig(config.Get()),
	}

	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to serialize backup")
		return
	}

	path, _ := backupFilePath(dir, id)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to write backup file")
		return
	}

	log.Info().Str("id", id).Str("path", path).Msg("Backup created")

	jsonResponse(w, http.StatusCreated, BackupMeta{
		ID:        id,
		Filename:  id + ".json",
		CreatedAt: now,
		Size:      int64(len(data)),
		Version:   config.GetVersion(),
		Remark:    req.Remark,
	})
}

// UploadBackup handles uploading a backup file
func (h *Handlers) UploadBackup(w http.ResponseWriter, r *http.Request) {
	dir, err := ensureBackupDir()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to create backup directory")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var bf BackupFile
	if err := json.Unmarshal(body, &bf); err != nil || bf.Meta.CreatedAt.IsZero() && bf.Config.Shelly.Devices == nil && bf.Config.Mappings == nil {
		// Try as plain BackupConfig
		var bc BackupConfig
		if err2 := json.Unmarshal(body, &bc); err2 != nil || (bc.Shelly.Devices == nil && bc.Mappings == nil && bc.Loxone.Miniservers == nil) {
			jsonError(w, http.StatusBadRequest, "invalid backup format")
			return
		}
		bf = BackupFile{
			Meta: BackupFileMeta{
				Version:   "imported",
				CreatedAt: time.Now(),
				Remark:    "Uploaded",
			},
			Config: bc,
		}
	}
	if err := config.Validate(backupToConfig(bf.Config)); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid backup: "+err.Error())
		return
	}
	body, _ = json.MarshalIndent(bf, "", "  ")

	id := fmt.Sprintf("upload-%s", time.Now().Format("20060102-150405"))
	path, _ := backupFilePath(dir, id)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to save uploaded backup")
		return
	}

	log.Info().Str("id", id).Msg("Backup uploaded")

	jsonResponse(w, http.StatusCreated, BackupMeta{
		ID:        id,
		Filename:  id + ".json",
		CreatedAt: bf.Meta.CreatedAt,
		Size:      int64(len(body)),
		Version:   bf.Meta.Version,
		Remark:    bf.Meta.Remark,
	})
}

// DeleteBackup deletes a backup file
func (h *Handlers) DeleteBackup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	path, ok := backupFilePath(config.BackupDir(), id)
	if !ok {
		jsonError(w, http.StatusBadRequest, "invalid backup id")
		return
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		jsonError(w, http.StatusNotFound, "backup not found")
		return
	}
	if err := os.Remove(path); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to delete backup")
		return
	}

	log.Info().Str("id", id).Msg("Backup deleted")
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func readBackup(id string) (*BackupFile, []byte, int, string) {
	path, ok := backupFilePath(config.BackupDir(), id)
	if !ok {
		return nil, nil, http.StatusBadRequest, "invalid backup id"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, http.StatusNotFound, "backup not found"
		}
		return nil, nil, http.StatusInternalServerError, "failed to read backup"
	}
	var bf BackupFile
	if err := json.Unmarshal(data, &bf); err != nil {
		return nil, nil, http.StatusInternalServerError, "failed to parse backup"
	}
	return &bf, data, http.StatusOK, ""
}

// DownloadBackup downloads a backup file
func (h *Handlers) DownloadBackup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	_, data, status, msg := readBackup(id)
	if status != http.StatusOK {
		jsonError(w, status, msg)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.json", id))
	if _, err := w.Write(data); err != nil {
		log.Debug().Err(err).Msg("Failed to write backup download")
	}
}

func backupToConfig(bc BackupConfig) *config.Config {
	return &config.Config{
		Server:   bc.Server,
		Shelly:   bc.Shelly,
		Loxone:   bc.Loxone,
		MQTT:     bc.MQTT,
		Logging:  bc.Logging,
		Mappings: bc.Mappings,
	}
}

// RestoreBackup restores a backup and applies it to all runtime components.
func (h *Handlers) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	bf, _, status, msg := readBackup(id)
	if status != http.StatusOK {
		jsonError(w, status, msg)
		return
	}

	previous := config.Get()
	cfg, err := config.Update(func(c *config.Config) error {
		restored := backupToConfig(bf.Config)
		// Keep the current listening address and API token so the UI stays reachable.
		restored.Server.Port = c.Server.Port
		restored.Server.Host = c.Server.Host
		if restored.Server.APIToken == "" {
			restored.Server.APIToken = c.Server.APIToken
		}
		*c = *restored
		return nil
	})
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid backup: "+err.Error())
		return
	}

	if err := config.Save(); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to save restored config")
		return
	}

	h.applyConfig(previous, cfg)

	log.Info().Str("id", id).Msg("Backup restored")
	jsonResponse(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Backup restored and applied.",
	})
}
