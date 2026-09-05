package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/geekychris/mac_transformer/internal/core"
	"github.com/geekychris/mac_transformer/internal/mcpsvc"
	"github.com/geekychris/mac_transformer/internal/plugin"
)

type Server struct {
	Queue    *core.Queue
	Registry *plugin.Registry
	Store    *core.Store

	server          *http.Server
	cachedScriptDir string
}

// Start binds and serves on 127.0.0.1:<port> from the Store's Config. Returns
// once the listener is ready or the bind failed; the actual serving happens
// in a background goroutine so the caller can keep going.
func (s *Server) Start() error {
	cfg := s.Store.Get()
	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	mux := s.routes()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
	}
	go s.server.Serve(ln)
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /transform", s.transform)
	mux.HandleFunc("GET /jobs", s.listJobs)
	mux.HandleFunc("GET /jobs/{id}", s.getJob)
	mux.HandleFunc("GET /jobs/{id}/artifacts/{name}", s.getArtifact)
	mux.HandleFunc("GET /plugins", s.listPlugins)
	mux.HandleFunc("POST /plugins/reload", s.reloadPlugins)
	mux.HandleFunc("GET /voices", s.listVoices)
	mux.HandleFunc("GET /events", s.eventStream)
	mux.HandleFunc("GET /config", s.getConfig)
	mux.HandleFunc("PUT /config", s.putConfig)

	// MCP endpoint — single-URL Streamable-HTTP transport (POST-only).
	mcp := &mcpsvc.Handler{Queue: s.Queue, Registry: s.Registry, Store: s.Store}
	mux.Handle("POST /mcp", mcp)

	// Reveal/open helpers: let the frontend hand a path back to the OS.
	mux.HandleFunc("POST /open", s.openPath)
	mux.HandleFunc("POST /reveal", s.revealPath)

	// Retroactive publish: apply Books / Music / tags to an existing job's
	// artifacts. Useful when you forgot to tick the box up front.
	mux.HandleFunc("POST /jobs/{id}/publish", s.publishJob)
	mux.HandleFunc("POST /jobs/{id}/cancel", s.cancelJob)
	mux.HandleFunc("POST /jobs/{id}/retry", s.retryJob)
	mux.HandleFunc("GET /search", s.search)
	mux.HandleFunc("GET /recipes", s.listRecipes)
	return withCORS(mux)
}

// scriptDir locates the bundled plugin directory (all bundled plugins
// share it). Cached after first call; the location doesn't move at runtime.
func (s *Server) scriptDir() string {
	if s.cachedScriptDir != "" {
		return s.cachedScriptDir
	}
	for _, lp := range s.Registry.All() {
		if lp.Manifest.Bundled {
			s.cachedScriptDir = lp.Manifest.Dir
			return s.cachedScriptDir
		}
	}
	return ""
}

// --- handlers ---

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	pending, running, completed, failed := s.queueCounts()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"version":         "0.1.0",
		"plugins":         len(s.Registry.All()),
		"port":            s.Store.Get().Port,
		"queue_pending":   pending,
		"queue_running":   running,
		"queue_completed": completed,
		"queue_failed":    failed,
	})
}

func (s *Server) queueCounts() (pending, running, completed, failed int) {
	for _, j := range s.Queue.List() {
		switch j.Status {
		case core.StatusPending:
			pending++
		case core.StatusRunning:
			running++
		case core.StatusCompleted:
			completed++
		case core.StatusFailed:
			failed++
		}
	}
	return
}

func (s *Server) transform(w http.ResponseWriter, r *http.Request) {
	var req core.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.InputPath == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	if !filepath.IsAbs(req.InputPath) {
		http.Error(w, "path must be absolute", http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(req.InputPath); err != nil {
		http.Error(w, "input not readable: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Apply named recipe, if any, before the queue sees it.
	if req.Recipe != "" {
		var found *core.Recipe
		for _, r := range s.Store.Get().Recipes {
			if r.Name == req.Recipe {
				r2 := r
				found = &r2
				break
			}
		}
		if found == nil {
			http.Error(w, "recipe not found: "+req.Recipe, http.StatusBadRequest)
			return
		}
		req = core.ApplyRecipe(req, *found)
	}
	if len(req.Outputs) == 0 {
		req.Outputs = []core.OutputKind{core.OutputPDF}
	}

	// Duplicate check: if this exact file content has already produced a
	// completed job, hand back that job instead of re-running. Callers
	// can force a re-run with `force: true`. The client sees a 200 (not
	// 202) with an `X-Duplicate-Of` header so it can differentiate.
	if !req.Force {
		if hash := core.HashInputForRequest(req.InputPath); hash != "" {
			if prior := s.Queue.FindByInputHash(hash); prior != nil && prior.Status == core.StatusCompleted {
				w.Header().Set("X-Duplicate-Of", prior.ID)
				writeJSON(w, http.StatusOK, prior)
				return
			}
		}
	}
	job := s.Queue.Submit(req)
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Queue.List())
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, ok := s.Queue.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) getArtifact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")
	job, ok := s.Queue.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	for _, a := range job.Artifacts {
		if filepath.Base(a.Path) == name {
			http.ServeFile(w, r, a.Path)
			return
		}
	}
	http.NotFound(w, r)
}

type pluginView struct {
	Name         string                `json:"name"`
	Description  string                `json:"description"`
	Capabilities []core.CapabilityView `json:"capabilities"`
	Available    bool                  `json:"available"`
	Reason       string                `json:"reason,omitempty"`
	Bundled      bool                  `json:"bundled"`
}

func (s *Server) listPlugins(w http.ResponseWriter, r *http.Request) {
	all := s.Registry.All()
	out := make([]pluginView, 0, len(all))
	for _, lp := range all {
		caps := make([]core.CapabilityView, 0, len(lp.Manifest.Capabilities))
		for _, c := range lp.Manifest.Capabilities {
			caps = append(caps, core.CapabilityView{From: c.From, To: c.To, Priority: c.Priority})
		}
		out = append(out, pluginView{
			Name:         lp.Manifest.Name,
			Description:  lp.Manifest.Description,
			Capabilities: caps,
			Available:    lp.Available,
			Reason:       lp.Reason,
			Bundled:      lp.Manifest.Bundled,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) reloadPlugins(w http.ResponseWriter, r *http.Request) {
	s.Registry.Reload(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"reloaded": true})
}

func (s *Server) listVoices(w http.ResponseWriter, r *http.Request) {
	engine := r.URL.Query().Get("engine")
	if engine == "" {
		engine = s.Store.Get().DefaultTTSEngine
	}
	switch engine {
	case "kokoro":
		voices, err := fetchKokoroVoices(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"engine": engine, "voices": voices})
	case "say":
		voices, err := macSayVoices(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"engine": engine, "voices": voices})
	default:
		http.Error(w, "unknown engine", http.StatusBadRequest)
	}
}

func (s *Server) eventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sub, cancel := s.Queue.Subscribe()
	defer cancel()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub:
			if !ok {
				return
			}
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, b)
			flusher.Flush()
		}
	}
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Store.Get())
}

// listRecipes returns the user's saved recipes so the DropZone can render
// a picker. Recipes are stored in Config; add/edit lives in Settings.
func (s *Server) listRecipes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Store.Get().Recipes)
}

// search scans every completed job's text/summary/markdown artifacts for
// the ?q= substring (case-insensitive) and returns matches with snippets.
// Read-only, cheap, no index maintained — since job history is capped at
// a few thousand entries this stays fast enough without a full index.
type searchHit struct {
	JobID   string `json:"job_id"`
	Path    string `json:"path"`
	Snippet string `json:"snippet"`
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "q is required", http.StatusBadRequest)
		return
	}
	qLower := strings.ToLower(q)
	var hits []searchHit

	for _, j := range s.Queue.List() {
		if j.Status != core.StatusCompleted {
			continue
		}
		for _, a := range j.Artifacts {
			if !isSearchable(a.Format) {
				continue
			}
			data, err := os.ReadFile(a.Path)
			if err != nil {
				continue
			}
			text := string(data)
			// Locate the first match, extract 80 chars either side.
			idx := strings.Index(strings.ToLower(text), qLower)
			if idx < 0 {
				continue
			}
			start := idx - 80
			if start < 0 {
				start = 0
			}
			end := idx + len(q) + 80
			if end > len(text) {
				end = len(text)
			}
			snippet := strings.ReplaceAll(text[start:end], "\n", " ")
			hits = append(hits, searchHit{JobID: j.ID, Path: a.Path, Snippet: snippet})
			if len(hits) >= 50 {
				break
			}
		}
		if len(hits) >= 50 {
			break
		}
	}
	writeJSON(w, http.StatusOK, hits)
}

func isSearchable(f core.Format) bool {
	switch f {
	case core.FormatTXT, "md", "markdown":
		return true
	}
	return false
}

// retryJob resubmits a completed-or-failed job to re-run only its failed
// steps. If all steps succeeded, this is a no-op (returns 409). The
// original job stays in place — a NEW job id is produced so history and
// artifacts don't get overwritten.
//
// Body (optional): {"kinds": ["audio"]} to explicitly limit which outputs
// to redo. If omitted, we re-run any step whose status is not "completed".
func (s *Server) retryJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	prior, ok := s.Queue.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Kinds []core.OutputKind `json:"kinds,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	kinds := body.Kinds
	if len(kinds) == 0 {
		// Default: whatever failed.
		for _, s := range prior.Steps {
			if s.Status != core.StepCompleted {
				kinds = append(kinds, s.Kind)
			}
		}
	}
	if len(kinds) == 0 {
		http.Error(w, "nothing to retry (all steps succeeded)", http.StatusConflict)
		return
	}

	next := prior.Request
	next.Outputs = kinds
	next.Force = true // don't hit duplicate detection

	job := s.Queue.Submit(next)
	w.Header().Set("X-Retry-Of", prior.ID)
	writeJSON(w, http.StatusAccepted, job)
}

// cancelJob cancels a pending or running job. Returns 404 if the id
// doesn't exist, 409 if the job is already finished.
func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, ok := s.Queue.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !s.Queue.Cancel(id) {
		http.Error(w, "job already finished", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// publishJobRequest is the retroactive publish payload. Any field left
// zero means "don't touch that target" — sending {"publish_to_books":true,
// "books_collection":"X"} adds the job's PDFs to Books collection X but
// does nothing for audio or tags.
type publishJobRequest struct {
	PublishToBooks       bool     `json:"publish_to_books,omitempty"`
	BooksCollection      string   `json:"books_collection,omitempty"`
	PublishToMusic       bool     `json:"publish_to_music,omitempty"`
	MusicPlaylist        string   `json:"music_playlist,omitempty"`
	MusicGenre           string   `json:"music_genre,omitempty"`
	PublishToReadingList bool     `json:"publish_to_reading_list,omitempty"`
	Tags                 []string `json:"tags,omitempty"`
}

// publishJob applies Books / Music / tags publishing to an existing job's
// artifacts. Blocking — returns after all applicable sinks have been
// invoked (or timed out). Emits a "progress" event so the SSE subscribers
// pick up the new publish results without polling.
func (s *Server) publishJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, ok := s.Queue.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if len(job.Artifacts) == 0 {
		http.Error(w, "job has no artifacts to publish", http.StatusBadRequest)
		return
	}

	var req publishJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !req.PublishToBooks && !req.PublishToMusic && !req.PublishToReadingList && len(req.Tags) == 0 {
		http.Error(w, "nothing to publish (set publish_to_books, publish_to_music, publish_to_reading_list, or tags)", http.StatusBadRequest)
		return
	}

	po := core.PublishOptions{
		Books:           req.PublishToBooks,
		BooksCollection: req.BooksCollection,
		Music:           req.PublishToMusic,
		MusicPlaylist:   req.MusicPlaylist,
		MusicGenre:      req.MusicGenre,
		ReadingList:     req.PublishToReadingList,
		Tags:            req.Tags,
	}
	dir := s.scriptDir()

	// Walk artifacts; publish helpers no-op on format mismatch, so we can
	// pass the same options and each sink picks the artifacts it wants.
	for i := range job.Artifacts {
		core.ApplyPostProcessing(r.Context(), &job.Artifacts[i], po, dir)
	}

	// Notify SSE subscribers so the queue view refreshes with the new
	// publish chips.
	s.Queue.EmitProgress(job)

	writeJSON(w, http.StatusOK, job)
}

// openPath hands a filesystem path to macOS `open`, which routes it through
// Launch Services — same as double-clicking the file in Finder.
func (s *Server) openPath(w http.ResponseWriter, r *http.Request) {
	path, err := readPathBody(r, s.Queue)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := exec.Command("open", path).Run(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"opened": path})
}

// revealPath opens Finder with the file selected but not opened.
func (s *Server) revealPath(w http.ResponseWriter, r *http.Request) {
	path, err := readPathBody(r, s.Queue)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := exec.Command("open", "-R", path).Run(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revealed": path})
}

// readPathBody pulls a `{"path": "..."}` payload and validates the path is
// one the app actually produced. Otherwise anyone with network access to
// 127.0.0.1:8771 could tell macOS to open arbitrary files.
func readPathBody(r *http.Request, q *core.Queue) (string, error) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	// Only allow paths that appear as artifacts in a known job. This is a
	// deliberate restriction — the endpoint isn't a general shell.
	for _, j := range q.List() {
		for _, a := range j.Artifacts {
			if a.Path == body.Path {
				return body.Path, nil
			}
		}
	}
	return "", fmt.Errorf("path is not an artifact of any known job")
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	var cfg core.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.Store.Set(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, s.Store.Get())
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func fetchKokoroVoices(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:8770/voices", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("kokoro /voices %d: %s", resp.StatusCode, string(body))
	}
	var payload struct {
		Voices []string `json:"voices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Voices, nil
}

func macSayVoices(ctx context.Context) ([]string, error) {
	out, err := exec.CommandContext(ctx, "say", "-v", "?").Output()
	if err != nil {
		return nil, err
	}
	var voices []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Lines look like: "Samantha            en_US    # ..."
		fields := strings.Fields(line)
		if len(fields) > 0 {
			voices = append(voices, fields[0])
		}
	}
	return voices, nil
}
