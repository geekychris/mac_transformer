package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/geekychris/mac_transformer/internal/app"
	"github.com/geekychris/mac_transformer/internal/core"
	"github.com/geekychris/mac_transformer/internal/httpapi"
	"github.com/geekychris/mac_transformer/internal/notify"
	"github.com/geekychris/mac_transformer/internal/plugin"
	"github.com/geekychris/mac_transformer/internal/watch"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed plugins
var bundledPlugins embed.FS

// Register the custom event pushed to the frontend when the OS hands us
// files (drag-drop into the window, or Finder "Open With").
func init() {
	application.RegisterEvent[[]string]("files-received")
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Seed PATH so plugin probes find Homebrew-installed tools even when
	// macOS launched us with the minimal Finder/LaunchServices PATH.
	// Must happen BEFORE plugin.Load so probes see the right binaries.
	seedPATH()

	// Bundled plugins ship inside the binary but tools like osascript need
	// them on disk. Unpack to Application Support on every launch so the
	// embedded copy is authoritative.
	bundledDir, err := unpackBundledPlugins()
	if err != nil {
		return err
	}

	store, err := core.LoadStore()
	if err != nil {
		log.Printf("config load warning: %v", err)
	}
	cfg := store.Get()

	ctx := context.Background()
	pluginDirs := []string{bundledDir, cfg.UserPluginDir}
	registry, loadErrs := plugin.Load(ctx, pluginDirs)
	for _, e := range loadErrs {
		log.Printf("plugin load: %v", e)
	}
	for _, lp := range registry.All() {
		if len(lp.Manifest.Dir) >= len(bundledDir) && lp.Manifest.Dir[:len(bundledDir)] == bundledDir {
			lp.Manifest.Bundled = true
		}
	}

	dispatcher := &core.Dispatcher{Registry: registry, Config: store}

	home, _ := os.UserHomeDir()
	history, err := core.NewHistory(filepath.Join(home, ".mac-transformer"))
	if err != nil {
		log.Printf("history: %v (continuing without persistence)", err)
	}
	queue := core.NewQueue(dispatcher, history)

	// Notification watcher: fires on job completion/failure/cancel.
	go (&notify.Watcher{Queue: queue, Config: store}).Run()

	// Filesystem watcher: submits jobs for files landing in configured folders.
	go watch.NewManager(queue, store).Run()

	httpServer := &httpapi.Server{
		Queue:    queue,
		Registry: registry,
		Store:    store,
	}
	if err := httpServer.Start(); err != nil {
		log.Printf("http server: %v (continuing; GUI still works)", err)
	} else {
		log.Printf("REST/MCP listening on 127.0.0.1:%d", cfg.Port)
	}

	svc := &app.TransformerService{Queue: queue, Registry: registry, Store: store}

	wailsApp := application.New(application.Options{
		Name:        "Mac Transformer",
		Description: "Convert docs to PDF / text / audio via a pluggable pipeline",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// Stay alive when the window closes — we're a menu-bar app.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	window := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "Mac Transformer",
		Width:          1100,
		Height:         700,
		EnableFileDrop: true, // hand dropped filenames to the WindowFilesDropped listener
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 44,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(20, 22, 30),
		URL:              "/",
	})

	// Drag-drop: forward absolute paths to the frontend as a custom event.
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		files := e.Context().DroppedFiles()
		if len(files) == 0 {
			return
		}
		wailsApp.Event.Emit("files-received", files)
	})

	// Finder "Open With" / `open -a` — the OS hands us one file at a time,
	// batched in short bursts. Forward each to the frontend; if the window
	// is hidden, show it so the user can see what's happening.
	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationOpenedWithFile, func(e *application.ApplicationEvent) {
		path := e.Context().Filename()
		if path == "" {
			return
		}
		if window != nil {
			window.Show()
			window.Focus()
		}
		wailsApp.Event.Emit("files-received", []string{path})
	})

	installAppMenu(wailsApp, svc, window)
	installTray(wailsApp, svc, window, queue)

	// Also accept file paths given on argv (drag onto Dock icon before the
	// app has finished launching queues them here rather than as events).
	if extra := extraArgPaths(); len(extra) > 0 {
		go func() {
			wailsApp.Event.Emit("files-received", extra)
		}()
	}

	return wailsApp.Run()
}

// unpackBundledPlugins copies the embedded plugins/ dir out to a stable
// per-user location so tools like osascript can read the script files by
// path. Overwrites on every launch — the embedded copy is authoritative.
func unpackBundledPlugins() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dst := filepath.Join(home, "Library", "Application Support", "MacTransformer", "plugins")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}
	entries, err := bundledPlugins.ReadDir("plugins")
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		src := "plugins/" + e.Name()
		data, err := bundledPlugins.ReadFile(src)
		if err != nil {
			return "", err
		}
		out := filepath.Join(dst, e.Name())
		mode := os.FileMode(0o644)
		if filepath.Ext(e.Name()) == ".sh" {
			mode = 0o755
		}
		if err := os.WriteFile(out, data, mode); err != nil {
			return "", err
		}
	}
	return dst, nil
}

// extraArgPaths returns command-line file arguments (absolute paths only).
// macOS will pass -psn_x_y as the first arg when launched from Finder; we
// filter those out.
func extraArgPaths() []string {
	var out []string
	for _, a := range os.Args[1:] {
		if len(a) > 5 && a[:5] == "-psn_" {
			continue
		}
		if filepath.IsAbs(a) {
			out = append(out, a)
		}
	}
	return out
}
