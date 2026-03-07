package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/loxone"
	"github.com/sbeyeler/loxone2shelly/internal/shelly"
)

// Server is the HTTP/WebSocket API server
type Server struct {
	router         *mux.Router
	httpServer     *http.Server
	wsHub          *WebSocketHub
	handlers       *Handlers
	shellyClient   *shelly.Client
	mappingManager *loxone.MappingManager
	udpSender      *loxone.UDPSender
	httpSender     *loxone.HTTPSender
	mqttSender     *loxone.MQTTSender
}

// NewServer creates a new API server
func NewServer(shellyClient *shelly.Client, mappingManager *loxone.MappingManager, udpSender *loxone.UDPSender, httpSender *loxone.HTTPSender, mqttSender *loxone.MQTTSender) *Server {
	s := &Server{
		shellyClient:   shellyClient,
		mappingManager: mappingManager,
		udpSender:      udpSender,
		httpSender:     httpSender,
		mqttSender:     mqttSender,
	}

	s.handlers = &Handlers{
		shellyClient:   shellyClient,
		mappingManager: mappingManager,
		udpSender:      udpSender,
		httpSender:     httpSender,
	}

	s.wsHub = NewWebSocketHub(shellyClient, mappingManager, udpSender, httpSender, mqttSender)

	s.router = mux.NewRouter()
	s.setupRoutes()

	return s
}

func (s *Server) setupRoutes() {
	// Middleware
	s.router.Use(corsMiddleware)
	s.router.Use(loggingMiddleware)

	// API routes
	api := s.router.PathPrefix("/api").Subrouter()

	// Health
	api.HandleFunc("/health", s.handlers.Health).Methods("GET", "OPTIONS")

	// Devices
	api.HandleFunc("/devices", s.handlers.GetDevices).Methods("GET", "OPTIONS")
	api.HandleFunc("/devices/{id}", s.handlers.GetDevice).Methods("GET", "OPTIONS")
	api.HandleFunc("/devices/{id}", s.handlers.SetDevice).Methods("PUT", "OPTIONS")

	// Inputs
	api.HandleFunc("/inputs", s.handlers.GetInputs).Methods("GET", "OPTIONS")

	// Mappings
	api.HandleFunc("/mappings", s.handlers.GetMappings).Methods("GET", "OPTIONS")
	api.HandleFunc("/mappings", s.handlers.CreateMapping).Methods("POST", "OPTIONS")
	api.HandleFunc("/mappings/{id}", s.handlers.UpdateMapping).Methods("PUT", "OPTIONS")
	api.HandleFunc("/mappings/{id}", s.handlers.DeleteMapping).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/mappings/export", s.handlers.ExportMappings).Methods("GET", "OPTIONS")
	api.HandleFunc("/mappings/import", s.handlers.ImportMappings).Methods("POST", "OPTIONS")

	// Config
	api.HandleFunc("/config", s.handlers.GetConfig).Methods("GET", "OPTIONS")
	api.HandleFunc("/config", s.handlers.UpdateConfig).Methods("PUT", "OPTIONS")

	// Backups
	api.HandleFunc("/backups", s.handlers.ListBackups).Methods("GET", "OPTIONS")
	api.HandleFunc("/backups", s.handlers.CreateBackup).Methods("POST", "OPTIONS")
	api.HandleFunc("/backups/upload", s.handlers.UploadBackup).Methods("POST", "OPTIONS")
	api.HandleFunc("/backups/{id}", s.handlers.DeleteBackup).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/backups/{id}/download", s.handlers.DownloadBackup).Methods("GET", "OPTIONS")
	api.HandleFunc("/backups/{id}/restore", s.handlers.RestoreBackup).Methods("POST", "OPTIONS")

	// UDP test
	api.HandleFunc("/udp/test", s.handlers.TestUDP).Methods("POST", "OPTIONS")

	// Export
	api.HandleFunc("/export/inputs", s.handlers.ExportVirtualInputs).Methods("GET", "OPTIONS")
	api.HandleFunc("/export/outputs", s.handlers.ExportVirtualOutputs).Methods("GET", "OPTIONS")

	// Logs
	api.HandleFunc("/logs", s.handlers.GetLogs).Methods("GET", "OPTIONS")

	// Shelly management
	api.HandleFunc("/shelly/discover", s.handlers.DiscoverShelly).Methods("GET", "OPTIONS")
	api.HandleFunc("/shelly/add", s.handlers.AddShelly).Methods("POST", "OPTIONS")
	api.HandleFunc("/shelly/{id}", s.handlers.RemoveShelly).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/shelly/{id}/test", s.handlers.TestShellyConnection).Methods("POST", "OPTIONS")

	// Swagger
	api.HandleFunc("/swagger.json", s.handlers.SwaggerJSON).Methods("GET", "OPTIONS")
	api.HandleFunc("/swagger", s.handlers.SwaggerUI).Methods("GET", "OPTIONS")

	// WebSocket
	s.router.HandleFunc("/ws", s.wsHub.HandleWebSocket)

	// Static files
	staticDir := "./web/dist"
	if _, err := os.Stat(staticDir); err == nil {
		spa := spaHandler{staticPath: staticDir, indexPath: "index.html"}
		s.router.PathPrefix("/").Handler(spa)
	}
}

// spaHandler serves a single-page application
type spaHandler struct {
	staticPath string
	indexPath  string
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := h.staticPath + r.URL.Path

	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		http.ServeFile(w, r, h.staticPath+"/"+h.indexPath)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.FileServer(http.Dir(h.staticPath)).ServeHTTP(w, r)
}

// Start starts the HTTP server
func (s *Server) Start(host string, port int) error {
	addr := fmt.Sprintf("%s:%d", host, port)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start WebSocket hub
	go s.wsHub.Run()

	log.Info().Str("addr", addr).Msg("Starting API server")
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	log.Info().Msg("Shutting down API server")

	s.wsHub.Stop()

	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// corsMiddleware handles CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs HTTP requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Skip logging for static files and WebSocket upgrades
		if strings.HasPrefix(r.URL.Path, "/assets/") || r.URL.Path == "/ws" {
			next.ServeHTTP(w, r)
			return
		}

		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)

		log.Debug().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", wrapped.statusCode).
			Dur("duration", time.Since(start)).
			Msg("HTTP request")
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
