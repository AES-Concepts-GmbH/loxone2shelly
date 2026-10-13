package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeShelly emulates a Gen2 two-channel switch (Pro 2PM) with one input.
type fakeShelly struct {
	mu      sync.Mutex
	outputs [2]bool
	power   float64
	calls   int
	srv     *httptest.Server
}

func newFakeShelly(t *testing.T) *fakeShelly {
	t.Helper()
	f := &fakeShelly{power: 4.2}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

// hostPort returns the "ip" to configure in the gateway.
func (f *fakeShelly) hostPort() string {
	return strings.TrimPrefix(f.srv.URL, "http://")
}

func (f *fakeShelly) output(i int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.outputs[i]
}

func (f *fakeShelly) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++

	b := func(v bool) string {
		if v {
			return "true"
		}
		return "false"
	}

	switch {
	case r.URL.Path == "/rpc/Shelly.GetDeviceInfo":
		_, _ = w.Write([]byte(`{"model":"SPSW-002PE16EU","fw_id":"20240101","app":"Pro2PM","gen":2}`))
	case r.URL.Path == "/rpc/Shelly.GetStatus":
		_, _ = w.Write([]byte(`{
			"switch:0":{"id":0,"output":` + b(f.outputs[0]) + `,"apower":` + jsonNumber(f.power) + `,"voltage":230,"current":0.02,"aenergy":{"total":10}},
			"switch:1":{"id":1,"output":` + b(f.outputs[1]) + `,"apower":0,"voltage":230,"current":0,"aenergy":{"total":0}},
			"input:0":{"id":0,"state":false}
		}`))
	case r.URL.Path == "/rpc" && r.Method == http.MethodPost:
		var req struct {
			Method string                 `json:"method"`
			Params map[string]interface{} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method != "Switch.Set" {
			_, _ = w.Write([]byte(`{"id":1,"error":{"code":404,"message":"No handler for ` + req.Method + `"}}`))
			return
		}
		id, _ := req.Params["id"].(float64)
		if int(id) < 0 || int(id) > 1 {
			_, _ = w.Write([]byte(`{"id":1,"error":{"code":-105,"message":"invalid id"}}`))
			return
		}
		if on, ok := req.Params["on"].(bool); ok {
			f.outputs[int(id)] = on
		}
		_, _ = w.Write([]byte(`{"id":1,"result":{"was_on":false}}`))
	default:
		http.NotFound(w, r)
	}
}

func jsonNumber(f float64) string {
	data, _ := json.Marshal(f)
	return string(data)
}
