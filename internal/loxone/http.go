package loxone

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/config"
)

// httpQueueSize bounds the number of pending requests per Miniserver.
const httpQueueSize = 256

// httpTarget represents a single HTTP connection to a Miniserver
type httpTarget struct {
	baseURL  string
	user     string
	password string
	sendAll  bool
	name     string
	client   *http.Client
	// queue serialises requests per Miniserver so an unreachable one only
	// delays its own feedback.
	queue *worker
}

// HTTPSender sends status updates to one or more Loxone Miniservers via HTTP
type HTTPSender struct {
	targets map[string]*httpTarget // key = MiniserverConfig.ID
	mu      sync.Mutex
}

// NewHTTPSender creates a new HTTP sender
func NewHTTPSender() *HTTPSender {
	return &HTTPSender{
		targets: make(map[string]*httpTarget),
	}
}

func newHTTPClient(insecure bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit opt-in per miniserver
	}
	return &http.Client{Timeout: 5 * time.Second, Transport: transport}
}

// Configure sets up or reconfigures all HTTP targets based on current miniserver config.
func (s *HTTPSender) Configure(miniservers []config.MiniserverConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closeTargetsLocked()

	for _, ms := range miniservers {
		if !ms.HTTPEnabled {
			continue
		}

		baseURL := ms.HTTPURL
		if baseURL == "" {
			if ms.IP == "" {
				continue
			}
			baseURL = fmt.Sprintf("http://%s", ms.IP)
		}

		s.targets[ms.ID] = &httpTarget{
			baseURL:  strings.TrimRight(baseURL, "/"),
			user:     ms.HTTPUser,
			password: ms.HTTPPassword,
			sendAll:  ms.SendAll,
			name:     ms.Name,
			client:   newHTTPClient(ms.HTTPInsecureTLS),
			queue:    newWorker("http:"+ms.ID, httpQueueSize),
		}

		log.Info().Str("id", ms.ID).Str("name", ms.Name).Str("url", baseURL).Bool("insecure_tls", ms.HTTPInsecureTLS).Msg("HTTP feedback configured for miniserver")
	}
}

func (s *HTTPSender) closeTargetsLocked() {
	for _, t := range s.targets {
		t.queue.close()
		t.client.CloseIdleConnections()
	}
	s.targets = make(map[string]*httpTarget)
}

// Close stops all queue workers.
func (s *HTTPSender) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeTargetsLocked()
}

// IsEnabled returns whether at least one HTTP target is active
func (s *HTTPSender) IsEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.targets) > 0
}

// HasSendAll returns true if at least one target has send_all enabled
func (s *HTTPSender) HasSendAll() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.targets {
		if t.sendAll {
			return true
		}
	}
	return false
}

// Send queues a single property update for a specific Miniserver. It never
// blocks; the request is performed by the target's worker.
func (s *HTTPSender) Send(miniserverID, loxoneID, property string, value interface{}) {
	s.mu.Lock()
	t, ok := s.targets[miniserverID]
	s.mu.Unlock()

	if !ok {
		return
	}

	s.enqueue(t, loxoneID, property, value)
}

// SendToAll queues a single property update for all Miniservers that have send_all enabled.
func (s *HTTPSender) SendToAll(loxoneID, property string, value interface{}) {
	s.mu.Lock()
	targets := make([]*httpTarget, 0)
	for _, t := range s.targets {
		if t.sendAll {
			targets = append(targets, t)
		}
	}
	s.mu.Unlock()

	for _, t := range targets {
		s.enqueue(t, loxoneID, property, value)
	}
}

func (s *HTTPSender) enqueue(t *httpTarget, loxoneID, property string, value interface{}) {
	t.queue.submit(func() { s.sendHTTP(t, loxoneID, property, value) })
}

// FeedbackURL builds the Loxone Virtual Input URL for a property update.
// Format: <base>/dev/sps/io/<loxone_id>_<property>/<value>
func FeedbackURL(baseURL, loxoneID, property string, value interface{}) string {
	viName := fmt.Sprintf("%s_%s", loxoneID, property)
	return fmt.Sprintf("%s/dev/sps/io/%s/%s",
		strings.TrimRight(baseURL, "/"),
		url.PathEscape(viName),
		url.PathEscape(fmt.Sprintf("%v", value)))
}

// sendHTTP performs the actual HTTP request to the Loxone Miniserver.
func (s *HTTPSender) sendHTTP(t *httpTarget, loxoneID, property string, value interface{}) {
	reqURL := FeedbackURL(t.baseURL, loxoneID, property, value)

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		log.Warn().Err(err).Str("miniserver", t.name).Str("url", reqURL).Msg("Failed to create HTTP request")
		return
	}

	if t.user != "" {
		req.SetBasicAuth(t.user, t.password)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		log.Warn().Err(err).Str("miniserver", t.name).Str("url", reqURL).Msg("Failed to send HTTP feedback")
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Warn().Str("miniserver", t.name).Str("url", reqURL).Int("status", resp.StatusCode).Msg("Miniserver rejected HTTP feedback")
		return
	}

	log.Debug().Str("miniserver", t.name).Str("loxone_id", loxoneID).Str("property", property).Interface("value", value).Int("status", resp.StatusCode).Msg("HTTP feedback sent")
}
