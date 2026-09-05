package app

import (
	"context"

	"github.com/geekychris/mac_transformer/internal/core"
	"github.com/geekychris/mac_transformer/internal/plugin"
)

// TransformerService is bound to the Wails frontend. Its exported methods
// become callable via generated TS bindings.
type TransformerService struct {
	Queue    *core.Queue
	Registry *plugin.Registry
	Store    *core.Store
}

// Health returns a quick app-status snapshot for the UI to render.
type Health struct {
	OK              bool   `json:"ok"`
	Version         string `json:"version"`
	Port            int    `json:"port"`
	PluginsTotal    int    `json:"plugins_total"`
	PluginsAvail    int    `json:"plugins_available"`
	KokoroReachable bool   `json:"kokoro_reachable"`
	// Live queue counts. Pending is jobs waiting to start, Running is jobs
	// currently executing, Completed/Failed are terminal states this
	// session. The tray label uses these; the UI shows Pending+Running as
	// "active" and the terminal counts as "processed".
	QueuePending   int `json:"queue_pending"`
	QueueRunning   int `json:"queue_running"`
	QueueCompleted int `json:"queue_completed"`
	QueueFailed    int `json:"queue_failed"`
}

func (s *TransformerService) GetHealth() Health {
	all := s.Registry.All()
	avail := 0
	kokoro := false
	for _, lp := range all {
		if lp.Available {
			avail++
			if lp.Manifest.Name == "kokoro-tts" {
				kokoro = true
			}
		}
	}
	pending, running, completed, failed := s.queueCounts()
	return Health{
		OK:              true,
		Version:         "0.1.0",
		Port:            s.Store.Get().Port,
		PluginsTotal:    len(all),
		PluginsAvail:    avail,
		KokoroReachable: kokoro,
		QueuePending:    pending,
		QueueRunning:    running,
		QueueCompleted:  completed,
		QueueFailed:     failed,
	}
}

func (s *TransformerService) queueCounts() (pending, running, completed, failed int) {
	for _, j := range s.Queue.List() {
		switch j.Status {
		case "pending":
			pending++
		case "running":
			running++
		case "completed":
			completed++
		case "failed":
			failed++
		}
	}
	return
}

// Transform is the primary entry point from the frontend. It's async: it
// returns a Job in Pending/Running state and the frontend follows progress
// via the event stream (or by polling GetJob).
func (s *TransformerService) Transform(req core.Request) *core.Job {
	return s.Queue.Submit(req)
}

func (s *TransformerService) ListJobs() []*core.Job {
	return s.Queue.List()
}

func (s *TransformerService) GetJob(id string) *core.Job {
	j, _ := s.Queue.Get(id)
	return j
}

// PluginView is what the settings UI renders.
type PluginView struct {
	Name         string                `json:"name"`
	Description  string                `json:"description"`
	Capabilities []core.CapabilityView `json:"capabilities"`
	Available    bool                  `json:"available"`
	Reason       string                `json:"reason,omitempty"`
	Bundled      bool                  `json:"bundled"`
}

func (s *TransformerService) ListPlugins() []PluginView {
	all := s.Registry.All()
	out := make([]PluginView, 0, len(all))
	for _, lp := range all {
		caps := make([]core.CapabilityView, 0, len(lp.Manifest.Capabilities))
		for _, c := range lp.Manifest.Capabilities {
			caps = append(caps, core.CapabilityView{From: c.From, To: c.To, Priority: c.Priority})
		}
		out = append(out, PluginView{
			Name:         lp.Manifest.Name,
			Description:  lp.Manifest.Description,
			Capabilities: caps,
			Available:    lp.Available,
			Reason:       lp.Reason,
			Bundled:      lp.Manifest.Bundled,
		})
	}
	return out
}

func (s *TransformerService) ReloadPlugins() {
	s.Registry.Reload(context.Background())
}

func (s *TransformerService) GetConfig() core.Config { return s.Store.Get() }

func (s *TransformerService) SetConfig(cfg core.Config) error { return s.Store.Set(cfg) }
