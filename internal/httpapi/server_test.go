package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geekychris/mac_transformer/internal/core"
	"github.com/geekychris/mac_transformer/internal/plugin"
)

// helper — spin up a Server with the minimum bits to serve read-only
// endpoints. The queue is real but empty; the registry has no plugins.
func newTestServer() *Server {
	reg, _ := plugin.Load(context.Background(), nil)
	store, _ := core.LoadStore()
	dispatcher := &core.Dispatcher{Registry: reg, Config: store}
	queue := core.NewQueue(dispatcher, nil)
	return &Server{Queue: queue, Registry: reg, Store: store}
}

func TestServer_Health(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(w.Body).Decode(&body)
	if body["ok"] != true {
		t.Errorf("no ok field: %v", body)
	}
	if _, ok := body["queue_pending"]; !ok {
		t.Errorf("no queue_pending field: %v", body)
	}
}

func TestServer_ListJobsEmpty(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest("GET", "/jobs", nil)
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
	if !strings.HasPrefix(strings.TrimSpace(w.Body.String()), "[") {
		t.Errorf("expected JSON array: %s", w.Body.String())
	}
}

func TestServer_TransformRejectsRelativePath(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest("POST", "/transform",
		bytes.NewReader([]byte(`{"path":"relative.md","outputs":["pdf"]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: %d, body: %s", w.Code, w.Body.String())
	}
}

func TestServer_CancelNotFound(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest("POST", "/jobs/nonexistent/cancel", nil)
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status: %d", w.Code)
	}
}
