package api

import (
	"context"
	"crypto/subtle"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/config"
	"github.com/sbeyeler/loxone2shelly/internal/loxone"
	"github.com/sbeyeler/loxone2shelly/internal/shelly"
)

// MQTTSubscriber is the part of the MQTT subscriber the API needs for
// applying configuration changes at runtime.
type MQTTSubscriber interface {
	SetDevices(devices []config.ShellyDeviceConfig)
	Restart(cfg config.MQTTConfig) error
	IsConnected() bool
}

// Deps bundles the runtime components the API server operates on.
type Deps struct {
	Shelly     *shelly.Client
	Mappings   *loxone.MappingManager
	UDP        *loxone.UDPSender
	HTTP       *loxone.HTTPSender
	MQTT       *loxone.MQTTSender
	Dispatcher *loxone.Dispatcher
	MQTTSub    MQTTSubscriber // may be nil
	// SetLogLevel applies a new log level at runtime; may be nil.
	SetLogLevel func(level string)
	// StaticDir holds the built web UI; empty disables static serving.
	StaticDir string
}

// Server is the HTTP/WebSocket API server
type Server struct {
	router     *mux.Router
	httpServer *http.Server
	wsHub      *WebSocketHub
	handlers   *Handlers
}

// NewServer creates a new API server
func NewServer(deps Deps) *Server {
	if deps.Dispatcher == nil {
		deps.Dispatcher = loxone.NewDispatcher(deps.Mappings, deps.UDP, deps.HTTP, deps.MQTT)
	}
	if deps.StaticDir == "" {
		deps.StaticDir = "./web/dist"
	}

	s := &Server{}
	s.wsHub = NewWebSocketHub(deps.Shelly, deps.Mappings, deps.Dispatcher)
	s.handlers = &Handlers{deps: deps, wsHub: s.wsHub}

	s.router = mux.NewRouter()
	s.setupRoutes(deps.StaticDir)
	return s
}

// Handler returns the root HTTP handler (useful for tests).
func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) setupRoutes(staticDir string) {
	s.router.Use(securityHeadersMiddleware)
	s.router.Use(corsMiddleware)
	s.router.Use(loggingMiddleware)

	api := s.router.PathPrefix("/api").Subrouter()

	// Unauthenticated endpoints
	api.HandleFunc("/health", s.handlers.Health).Methods("GET", "OPTIONS")
	api.HandleFunc("/webhook", s.handlers.Webhook).Methods("GET", "POST", "OPTIONS")
	api.HandleFunc("/swagger.json", s.handlers.SwaggerJSON).Methods("GET", "OPTIONS")
	api.HandleFunc("/swagger", s.handlers.SwaggerUI).Methods("GET", "OPTIONS")

	// Authenticated endpoints
	auth := api.NewRoute().Subrouter()
	auth.Use(authMiddleware)

	auth.HandleFunc("/devices", s.handlers.GetDevices).Methods("GET", "OPTIONS")
	auth.HandleFunc("/devices/{id}", s.handlers.GetDevice).Methods("GET", "OPTIONS")
	auth.HandleFunc("/devices/{id}", s.handlers.SetDevice).Methods("PUT", "OPTIONS")

	auth.HandleFunc("/inputs", s.handlers.GetInputs).Methods("GET", "OPTIONS")

	auth.HandleFunc("/mappings", s.handlers.GetMappings).Methods("GET", "OPTIONS")
	auth.HandleFunc("/mappings", s.handlers.CreateMapping).Methods("POST", "OPTIONS")
	auth.HandleFunc("/mappings/export", s.handlers.ExportMappings).Methods("GET", "OPTIONS")
	auth.HandleFunc("/mappings/import", s.handlers.ImportMappings).Methods("POST", "OPTIONS")
	auth.HandleFunc("/mappings/{id}", s.handlers.UpdateMapping).Methods("PUT", "OPTIONS")
	auth.HandleFunc("/mappings/{id}", s.handlers.DeleteMapping).Methods("DELETE", "OPTIONS")

	auth.HandleFunc("/config", s.handlers.GetConfig).Methods("GET", "OPTIONS")
	auth.HandleFunc("/config", s.handlers.UpdateConfig).Methods("PUT", "OPTIONS")

	auth.HandleFunc("/backups", s.handlers.ListBackups).Methods("GET", "OPTIONS")
	auth.HandleFunc("/backups", s.handlers.CreateBackup).Methods("POST", "OPTIONS")
	auth.HandleFunc("/backups/upload", s.handlers.UploadBackup).Methods("POST", "OPTIONS")
	auth.HandleFunc("/backups/{id}", s.handlers.DeleteBackup).Methods("DELETE", "OPTIONS")
	auth.HandleFunc("/backups/{id}/download", s.handlers.DownloadBackup).Methods("GET", "OPTIONS")
	auth.HandleFunc("/backups/{id}/restore", s.handlers.RestoreBackup).Methods("POST", "OPTIONS")

	auth.HandleFunc("/udp/test", s.handlers.TestUDP).Methods("POST", "OPTIONS")

	auth.HandleFunc("/export/inputs", s.handlers.ExportVirtualInputs).Methods("GET", "OPTIONS")
	auth.HandleFunc("/export/outputs", s.handlers.ExportVirtualOutputs).Methods("GET", "OPTIONS")

	auth.HandleFunc("/logs", s.handlers.GetLogs).Methods("GET", "OPTIONS")

	auth.HandleFunc("/shelly/discover", s.handlers.DiscoverShelly).Methods("GET", "OPTIONS")
	auth.HandleFunc("/shelly/add", s.handlers.AddShelly).Methods("POST", "OPTIONS")
	auth.HandleFunc("/shelly/{id}", s.handlers.UpdateShelly).Methods("PUT", "OPTIONS")
	auth.HandleFunc("/shelly/{id}", s.handlers.RemoveShelly).Methods("DELETE", "OPTIONS")
	auth.HandleFunc("/shelly/{id}/test", s.handlers.TestShellyConnection).Methods("POST", "OPTIONS")

	// WebSocket (token checked inside the handler so browsers can pass it as query parameter)
	s.router.Handle("/ws", authMiddleware(http.HandlerFunc(s.wsHub.HandleWebSocket)))

	// Static files
	if _, err := os.Stat(staticDir); err == nil {
		s.router.PathPrefix("/").Handler(spaHandler{staticPath: staticDir, indexPath: "index.html"})
	}
}

// spaHandler serves a single-page application
type spaHandler struct {
	staticPath string
	indexPath  string
}

// ingressPathPattern accepts the path prefix a reverse proxy reports in
// X-Ingress-Path (Home Assistant: /api/hassio_ingress/<token>).
var ingressPathPattern = regexp.MustCompile(`^/[A-Za-z0-9_./-]{1,256}$`)

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rel := filepath.Clean("/" + r.URL.Path)
	path := filepath.Join(h.staticPath, rel)

	info, err := os.Stat(path)
	if err != nil || info.IsDir() || rel == "/"+h.indexPath {
		h.serveIndex(w, r)
		return
	}

	http.FileServer(http.Dir(h.staticPath)).ServeHTTP(w, r)
}

// serveIndex serves the SPA entry point with a <base href>, so the relative
// asset, API and WebSocket URLs of the UI resolve both at the root and behind
// a path prefix such as the Home Assistant ingress proxy.
func (h spaHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(filepath.Join(h.staticPath, h.indexPath))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	base := "/"
	if p := r.Header.Get("X-Ingress-Path"); ingressPathPattern.MatchString(p) {
		base = strings.TrimRight(p, "/") + "/"
	}
	page := strings.Replace(string(data), "<head>", `<head><base href="`+html.EscapeString(base)+`">`, 1)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(page))
}

// Start starts the HTTP server
func (s *Server) Start(host string, port int) error {
	addr := fmt.Sprintf("%s:%d", host, port)

	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

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

// --- Middleware ---

// extractToken reads the API token from the request.
func extractToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if k := r.Header.Get("X-API-Key"); k != "" {
		return k
	}
	return r.URL.Query().Get("token")
}

func tokenMatches(provided, expected string) bool {
	return expected != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// authMiddleware enforces the API token when one is configured.
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		expected := config.GetServer().APIToken
		if expected == "" {
			next.ServeHTTP(w, r)
			return
		}
		if !tokenMatches(extractToken(r), expected) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="loxone2shelly"`)
			jsonError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// originAllowed reports whether a browser origin may use the API.
// Same-origin requests (also behind a reverse proxy) and configured origins
// are allowed.
func originAllowed(r *http.Request, origin string) bool {
	if origin == "" {
		return true
	}
	// Requests proxied by the Home Assistant ingress carry the user's HA
	// origin; HA has already authenticated them. Browsers cannot set this
	// header themselves.
	if r.Header.Get("X-Ingress-Path") != "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) || strings.EqualFold(u.Host, r.Header.Get("X-Forwarded-Host")) {
		return true
	}
	for _, allowed := range config.GetServer().CORSOrigins {
		if allowed == "*" || strings.EqualFold(strings.TrimRight(allowed, "/"), origin) {
			return true
		}
	}
	return false
}

// corsMiddleware emits CORS headers only for allowed origins.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && originAllowed(r, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
			w.Header().Set("Access-Control-Max-Age", "600")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "no-referrer")
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
