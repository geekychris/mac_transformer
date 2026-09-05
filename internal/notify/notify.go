// Package notify posts native macOS notifications for job lifecycle events.
// Subscribes to the core.Queue event stream; renders a banner via
// `osascript display notification`. Silently skips if no notification
// permission has been granted.
package notify

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geekychris/mac_transformer/internal/core"
)

type Watcher struct {
	Queue  *core.Queue
	Config *core.Store
}

func (w *Watcher) Run() {
	sub, cancel := w.Queue.Subscribe()
	defer cancel()
	for ev := range sub {
		if !w.Config.Get().NotificationsEnabled {
			continue
		}
		switch ev.Type {
		case "completed":
			w.postCompleted(ev.Job)
		case "failed":
			w.postFailed(ev.Job)
		case "cancelled":
			w.postCancelled(ev.Job)
		}
	}
}

func (w *Watcher) postCompleted(job *core.Job) {
	stem := stemOf(job.Request.InputPath)
	title := "Mac Transformer"
	subtitle := stem + " · " + summariseArtifacts(job)
	body := "Click the app to view details."
	post(title, subtitle, body)
}

func (w *Watcher) postFailed(job *core.Job) {
	stem := stemOf(job.Request.InputPath)
	msg := firstNonEmpty(job.Message, "one or more steps failed")
	post("Mac Transformer · Failed", stem, msg)
}

func (w *Watcher) postCancelled(job *core.Job) {
	stem := stemOf(job.Request.InputPath)
	post("Mac Transformer · Cancelled", stem, "Job stopped.")
}

// post fires an AppleScript notification. Fails silently — if the user has
// declined banners in System Settings, there's nothing to do.
func post(title, subtitle, body string) {
	script := fmt.Sprintf(`display notification %q with title %q subtitle %q`,
		body, title, subtitle)
	_ = exec.Command("osascript", "-e", script).Run()
}

func stemOf(p string) string {
	base := filepath.Base(p)
	if i := strings.LastIndexByte(base, '.'); i > 0 {
		return base[:i]
	}
	return base
}

func summariseArtifacts(job *core.Job) string {
	if len(job.Artifacts) == 0 {
		return "no artifacts"
	}
	kinds := make(map[core.OutputKind]int)
	for _, a := range job.Artifacts {
		kinds[a.Kind]++
	}
	parts := []string{}
	for _, k := range []core.OutputKind{core.OutputPDF, core.OutputText, core.OutputAudio} {
		if n := kinds[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d artifacts", len(job.Artifacts))
	}
	return strings.Join(parts, ", ")
}

func firstNonEmpty(strs ...string) string {
	for _, s := range strs {
		if s != "" {
			return s
		}
	}
	return ""
}
