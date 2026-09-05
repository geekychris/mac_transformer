// Package watch subscribes to filesystem events on user-configured folders
// and submits transform jobs for each new (or modified) file.
package watch

import (
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/geekychris/mac_transformer/internal/core"
)

type Manager struct {
	Queue *core.Queue
	Store *core.Store

	mu       sync.Mutex
	watcher  *fsnotify.Watcher
	watching map[string]core.WatchFolder // path -> folder config

	// Debounce state: filesystem writes to a single file typically fire
	// multiple CREATE/WRITE events. We wait `debounce` after the last
	// event for a given path before actually submitting.
	pendingMu sync.Mutex
	pending   map[string]*time.Timer
	debounce  time.Duration
}

func NewManager(q *core.Queue, store *core.Store) *Manager {
	return &Manager{
		Queue:    q,
		Store:    store,
		watching: map[string]core.WatchFolder{},
		pending:  map[string]*time.Timer{},
		debounce: 1500 * time.Millisecond,
	}
}

// Run boots the fsnotify watcher and reconciles it against Config on every
// config change (polled). Runs until Stop is called.
func (m *Manager) Run() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("watch: fsnotify unavailable: %v", err)
		return
	}
	m.watcher = w
	defer w.Close()

	m.reconcile()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			m.handleEvent(ev)
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			log.Printf("watch: %v", err)
		case <-ticker.C:
			m.reconcile()
		}
	}
}

// reconcile compares the currently-watched dirs against Config.WatchFolders
// and adds/removes fsnotify paths so they match.
func (m *Manager) reconcile() {
	m.mu.Lock()
	defer m.mu.Unlock()

	want := map[string]core.WatchFolder{}
	for _, wf := range m.Store.Get().WatchFolders {
		if wf.Enabled && wf.Path != "" {
			want[wf.Path] = wf
		}
	}
	// Remove stale
	for path := range m.watching {
		if _, ok := want[path]; !ok {
			_ = m.watcher.Remove(path)
			delete(m.watching, path)
		}
	}
	// Add new
	for path, wf := range want {
		if _, ok := m.watching[path]; ok {
			continue
		}
		if err := m.watcher.Add(path); err != nil {
			log.Printf("watch: add %s: %v", path, err)
			continue
		}
		m.watching[path] = wf
	}
}

func (m *Manager) handleEvent(ev fsnotify.Event) {
	if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
		return
	}
	// Ignore hidden files, temp files, and our own output directories.
	base := filepath.Base(ev.Name)
	if strings.HasPrefix(base, ".") || strings.HasSuffix(base, "~") {
		return
	}
	// Filter to accepted extensions.
	dir := filepath.Dir(ev.Name)
	m.mu.Lock()
	wf, ok := m.watching[dir]
	m.mu.Unlock()
	if !ok {
		return
	}
	if !acceptExtension(base, wf.Extensions) {
		return
	}
	m.schedule(ev.Name, wf)
}

func (m *Manager) schedule(path string, wf core.WatchFolder) {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	if t, ok := m.pending[path]; ok {
		t.Stop()
	}
	m.pending[path] = time.AfterFunc(m.debounce, func() {
		m.pendingMu.Lock()
		delete(m.pending, path)
		m.pendingMu.Unlock()
		m.submit(path, wf)
	})
}

func (m *Manager) submit(path string, wf core.WatchFolder) {
	req := core.Request{InputPath: path}
	if wf.Recipe != "" {
		req.Recipe = wf.Recipe
		// Look up recipe and apply — same code path as the /transform
		// handler does, minimally duplicated here to avoid a cyclic import.
		for _, r := range m.Store.Get().Recipes {
			if r.Name == wf.Recipe {
				req = core.ApplyRecipe(req, r)
				break
			}
		}
	}
	m.Queue.Submit(req)
	log.Printf("watch: submitted %s via recipe %q", path, wf.Recipe)
}

func acceptExtension(name string, allowed []string) bool {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if len(allowed) == 0 {
		// Default: accept anything we have a converter for.
		switch ext {
		case "docx", "doc", "pptx", "ppt", "rtf", "pdf", "odt", "odp", "ods",
			"md", "markdown", "adoc", "asciidoc", "rst", "org", "html", "htm", "txt":
			return true
		}
		return false
	}
	for _, e := range allowed {
		if strings.EqualFold(strings.TrimPrefix(e, "."), ext) {
			return true
		}
	}
	return false
}
