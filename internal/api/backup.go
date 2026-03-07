package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/config"
	"github.com/sbeyeler/loxone2shelly/internal/models"
)

const backupDir = "backups"

// BackupConfig represents the structure of a backup file
type BackupConfig struct {
	Server   config.ServerConfig  `json:"server"`
	Shelly   config.ShellyConfig  `json:"shelly"`
	Loxone   config.LoxoneConfig  `json:"loxone"`
	Logging  config.LoggingConfig `json:"logging"`
	Mappings []models.Mapping     `json:"mappings"`
}

// BackupMeta holds metadata about a backup file
type BackupMeta struct {
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	CreatedAt time.Time `json:"created_at"`
	Size      int64     `json:"size"`
	Version   string    `json:"version,omitempty"`
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
}

func ensureBackupDir() error {
	return os.MkdirAll(backupDir, 0755)
}

func backupFilePath(id string) string {
	return filepath.Join(backupDir, id+".json")
}

// ListBackups returns all available backups
func (h *Handlers) ListBackups(w http.ResponseWriter, r *http.Request) {
	if err := ensureBackupDir(); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to access backup directory")
		return
	}

	entries, err := os.ReadDir(backupDir)
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

		// Try to read version from backup file
		data, err := os.ReadFile(filepath.Join(backupDir, entry.Name()))
		if err == nil {
			var bf BackupFile
			if json.Unmarshal(data, &bf) == nil {
				meta.Version = bf.Meta.Version
				if !bf.Meta.CreatedAt.IsZero() {
					meta.CreatedAt = bf.Meta.CreatedAt
				}
			}
		}

		backups = append(backups, meta)
	}

	// Sort by creation time, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	jsonResponse(w, http.StatusOK, backups)
}

// CreateBackup creates a new backup of the current configuration
func (h *Handlers) CreateBackup(w http.ResponseWriter, r *http.Request) {
	if err := ensureBackupDir(); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to create backup directory")
		return
	}

	cfg := config.Get()
	now := time.Now()
	id := now.Format("20060102-150405")

	backup := BackupFile{
		Meta: BackupFileMeta{
			Version:   config.GetVersion(),
			CreatedAt: now,
		},
		Config: BackupConfig{
			Server:   cfg.Server,
			Shelly:   cfg.Shelly,
			Loxone:   cfg.Loxone,
			Logging:  cfg.Logging,
			Mappings: cfg.Mappings,
		},
	}

	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to serialize backup")
		return
	}

	path := backupFilePath(id)
	if err := os.WriteFile(path, data, 0644); err != nil {
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
	})
}

// UploadBackup handles uploading a backup file
func (h *Handlers) UploadBackup(w http.ResponseWriter, r *http.Request) {
	if err := ensureBackupDir(); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to create backup directory")
		return
	}

	// Limit upload size to 10MB
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	// Validate it's a valid backup
	var bf BackupFile
	if err := json.Unmarshal(body, &bf); err != nil {
		// Try as plain BackupConfig
		var bc BackupConfig
		if err2 := json.Unmarshal(body, &bc); err2 != nil {
			jsonError(w, http.StatusBadRequest, "invalid backup format")
			return
		}
		bf = BackupFile{
			Meta: BackupFileMeta{
				Version:   "imported",
				CreatedAt: time.Now(),
			},
			Config: bc,
		}
		body, _ = json.MarshalIndent(bf, "", "  ")
	}

	id := fmt.Sprintf("upload-%s", time.Now().Format("20060102-150405"))
	path := backupFilePath(id)

	if err := os.WriteFile(path, body, 0644); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to save uploaded backup")
		return
	}

	log.Info().Str("id", id).Msg("Backup uploaded")

	jsonResponse(w, http.StatusCreated, BackupMeta{
		ID:        id,
		Filename:  id + ".json",
		CreatedAt: time.Now(),
		Size:      int64(len(body)),
		Version:   bf.Meta.Version,
	})
}

// DeleteBackup deletes a backup file
func (h *Handlers) DeleteBackup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	path := backupFilePath(id)
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

// DownloadBackup downloads a backup file
func (h *Handlers) DownloadBackup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	path := backupFilePath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			jsonError(w, http.StatusNotFound, "backup not found")
		} else {
			jsonError(w, http.StatusInternalServerError, "failed to read backup")
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.json", id))
	w.Write(data)
}

// RestoreBackup restores a backup
func (h *Handlers) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	path := backupFilePath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			jsonError(w, http.StatusNotFound, "backup not found")
		} else {
			jsonError(w, http.StatusInternalServerError, "failed to read backup")
		}
		return
	}

	var bf BackupFile
	if err := json.Unmarshal(data, &bf); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to parse backup")
		return
	}

	// Restore configuration
	config.RestoreFromBackup(
		bf.Config.Server,
		bf.Config.Shelly,
		bf.Config.Loxone,
		bf.Config.Logging,
		bf.Config.Mappings,
	)

	if err := config.Save(); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to save restored config")
		return
	}

	// Reload mapping manager
	h.mappingManager.Load(bf.Config.Mappings)

	// Reconfigure senders
	h.udpSender.Configure(bf.Config.Loxone.Miniservers)
	h.httpSender.Configure(bf.Config.Loxone.Miniservers)

	log.Info().Str("id", id).Msg("Backup restored")
	jsonResponse(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Backup restored. Restart recommended to apply all changes.",
	})
}
