package main

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/geekychris/mac_transformer/internal/app"
	"github.com/geekychris/mac_transformer/internal/core"
)

// installTray adds a persistent menu-bar icon with quick actions and a live
// counter of queued / running / completed jobs. The counter updates in real
// time by subscribing to the core.Queue event stream.
func installTray(a *application.App, svc *app.TransformerService, win *application.WebviewWindow, queue *core.Queue) {
	tray := a.SystemTray.New()

	menu := a.Menu.New()

	openItem := menu.Add("Open Mac Transformer")
	openItem.OnClick(func(_ *application.Context) {
		if win != nil {
			win.Show()
			win.Focus()
		}
	})

	menu.AddSeparator()

	// Counter row — updated by the goroutine below on every queue event.
	counterItem := menu.Add("No jobs yet")
	counterItem.SetEnabled(false)

	pluginItem := menu.Add("Plugins: —")
	pluginItem.SetEnabled(false)

	menu.AddSeparator()

	menu.Add("Reload plugins").OnClick(func(_ *application.Context) {
		svc.ReloadPlugins()
	})

	menu.AddSeparator()

	menu.Add("Quit").OnClick(func(_ *application.Context) {
		a.Quit()
	})

	tray.SetMenu(menu)

	// Initial render before the first event arrives.
	refresh := func() {
		h := svc.GetHealth()
		tray.SetLabel(trayLabel(h))
		counterItem.SetLabel(counterText(h))
		pluginItem.SetLabel(fmt.Sprintf("Plugins: %d/%d available", h.PluginsAvail, h.PluginsTotal))
	}
	refresh()

	// Subscribe to queue events and refresh on every state change. This
	// covers "queued", "started", "progress", "completed", "failed".
	go func() {
		sub, cancel := queue.Subscribe()
		defer cancel()
		for range sub {
			refresh()
		}
	}()
}

// trayLabel is the short string that shows in the menu bar strip itself.
// Kept minimal so it doesn't hog space:
//   idle:            "MT"
//   1 job running:   "MT ●"
//   3 active total:  "MT · 3"
//   1 running + 2 pending: "MT · 3"
func trayLabel(h app.Health) string {
	active := h.QueuePending + h.QueueRunning
	if active == 0 {
		return "MT"
	}
	if active == 1 && h.QueueRunning == 1 {
		return "MT ●"
	}
	return fmt.Sprintf("MT · %d", active)
}

// counterText is the more verbose row inside the tray's dropdown menu.
// Shows both live activity and cumulative processed counts.
func counterText(h app.Health) string {
	active := h.QueuePending + h.QueueRunning
	done := h.QueueCompleted
	failed := h.QueueFailed
	switch {
	case active == 0 && done == 0 && failed == 0:
		return "No jobs yet"
	case active == 0:
		if failed == 0 {
			return fmt.Sprintf("Idle · %d completed", done)
		}
		return fmt.Sprintf("Idle · %d completed · %d failed", done, failed)
	default:
		msg := fmt.Sprintf("%d active", active)
		if h.QueueRunning > 0 && h.QueuePending > 0 {
			msg = fmt.Sprintf("%d running · %d queued", h.QueueRunning, h.QueuePending)
		}
		if done > 0 || failed > 0 {
			msg = fmt.Sprintf("%s · %d done", msg, done)
			if failed > 0 {
				msg = fmt.Sprintf("%s · %d failed", msg, failed)
			}
		}
		return msg
	}
}
