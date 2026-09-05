# Architecture

Mac Transformer is a single Wails v3 process that hosts:

1. A **REST + MCP HTTP server** on `127.0.0.1:8771`
2. A **plugin registry** of format converters (YAML-declared, hot-loaded)
3. A **job queue** with persistence
4. Post-processing **sinks** (Books, Music, Reading List, Obsidian, Notion, Finder tags)
5. A **Wails webview UI** (React + TypeScript)
6. A **tray icon** with a live counter
7. A **filesystem watcher** for watch folders
8. A **notification watcher** for job-done banners

All in one binary. Nothing else runs unless the user launches Kokoro separately.

## Directory layout

```
main.go                       // Wails app entry: wires everything together
tray.go                       // macOS menu-bar tray + counter
menu.go                       // Native macOS app menu (File/Edit/View/…)
internal/
  core/                       // Domain: jobs, artifacts, plugins-agnostic pipeline
    job.go                    // Job, Request, Artifact, StepReport types
    config.go                 // Persisted user settings
    format.go                 // Extension → Format detection, DesiredFormat
    dispatcher.go             // The pipeline: input → converter chain → sinks
    queue.go                  // FIFO queue + cancel + event subscribers
    history.go                // ~/.mac-transformer/jobs.jsonl persistence
    publish.go                // Books/Music/Reading-List/Obsidian/Notion/tags
    chapters.go               // Heading detection for chapter-split audio
    cover.go                  // PNG cover art generation
    id3.go                    // ffmpeg-based audio metadata tagging
    pdfmeta.go                // pdfcpu-based PDF Info dict stamping
    summarize.go              // OpenAI-compatible chat completions client
    recipe.go                 // Saved request templates
    watch.go                  // WatchFolder struct (impl in internal/watch)

  plugin/                     // Plugin system: manifests, loading, execution
    manifest.go               // Manifest schema (YAML)
    loader.go                 // Registry + Match(from→to)
    probe.go                  // "Is this plugin available?" checks
    executor.go               // Run a plugin (command or http)
    template.go               // {{input}} / {{output_ext}} / …

  httpapi/                    // REST server
    server.go                 // All endpoints

  mcpsvc/                     // MCP JSON-RPC-over-HTTP handler
    server.go                 // initialize, tools/list, tools/call
    voices.go                 // Voice enumeration for Kokoro/say

  app/                        // Wails service bindings for the frontend
    service.go                // TransformerService — exposed to JS

  notify/                     // Job-done banner poster (osascript)
  watch/                      // Filesystem-watch daemon (fsnotify + debounce)

plugins/                      // Bundled plugin manifests + scripts
  microsoft-word-macos.yaml   // PDF via Word AppleScript
  microsoft-powerpoint-macos.yaml
  libreoffice-headless.yaml
  pandoc-pdf.yaml
  pandoc-text.yaml
  asciidoctor-pdf.yaml
  kokoro-tts.yaml
  mac-say-tts.yaml
  ocr-pdf.yaml                // Vision framework OCR
  pdftotext.yaml
  pdf-passthrough.yaml
  txt-passthrough.yaml
  md-adoc-passthrough.yaml
  *.applescript / *.sh        // Helper scripts referenced by manifests

frontend/                     // Wails webview
  src/
    App.tsx                   // Root — tabs, search box, event stream
    api.ts                    // REST client + TS types
    views/
      DropZone.tsx            // Submit form
      QueueView.tsx           // Job list with expandable per-step progress
      SettingsView.tsx        // Config editor
      PluginsView.tsx         // Loaded plugins + availability
      AudioPlayer.tsx         // Inline audio player + waveform
      PublishForm.tsx         // Retroactive publish per-job
      RecipeForm.tsx          // Recipe editor (form-based)

automator/MacTransformer.workflow/  // Automator Service (Finder Quick Action + share)
build/darwin/Info.plist       // Bundle Info.plist (CFBundleDocumentTypes, UTIs)
cmd/mt/                       // CLI companion
cmd/headless/                 // REST-only variant for testing
scripts/
  install.sh                  // Build .app + install to /Applications
  install-deps.sh             // brew-install runtime deps
```

## Request lifecycle

```
                         POST /transform
                              │
                              ▼
              ┌──────────────────────────────┐
              │ httpapi.server.transform     │
              │  1. validate body            │
              │  2. apply recipe (if any)    │
              │  3. dup-detect by hash       │
              │  4. Queue.Submit             │
              └───────────────┬──────────────┘
                              │
                              ▼
              ┌──────────────────────────────┐
              │      core.Queue              │
              │  (FIFO, single worker,       │
              │   emits events on transitions)│
              └───────────────┬──────────────┘
                              │
                              ▼
              ┌──────────────────────────────┐
              │  core.Dispatcher.Run(ctx)    │
              │                              │
              │  for each requested output:  │
              │    ┌────────────────────┐    │
              │    │ pickPlugins(from,  │    │
              │    │   to, overrides)   │    │
              │    └─────────┬──────────┘    │
              │              ▼               │
              │    ┌────────────────────┐    │
              │    │ Direct? try each   │    │
              │    │  in priority order │    │
              │    │ Chain? txt → tts   │    │
              │    │  for audio         │    │
              │    └─────────┬──────────┘    │
              │              ▼               │
              │    ┌────────────────────┐    │
              │    │ plugin.Execute     │    │
              │    │  (command / http)  │    │
              │    └─────────┬──────────┘    │
              │              ▼               │
              │    ┌────────────────────┐    │
              │    │ Post-process:      │    │
              │    │  ID3/cover/PDFmeta │    │
              │    │  Publish sinks     │    │
              │    │  Finder tags       │    │
              │    └────────────────────┘    │
              └───────────────┬──────────────┘
                              │
                              ▼
                    history.Append(job)
                    Event → SSE subscribers
                    Event → tray label update
                    Event → notification banner
```

## Plugin system

Every converter is a YAML manifest declaring:

- **capabilities**: `from → to` format pairs with a priority (higher wins)
- **requires**: availability probes (`binary`, `app-bundle`, `http`, `gem`, `file`)
- **run**: either `command` (spawn a subprocess) or `http` (POST/GET to a URL)

At startup, `plugin.Load` scans the bundled dir and the user dir (`~/.mac-transformer/plugins/`), parses every manifest, runs the probes once, and stores the registry with each plugin marked available/unavailable.

At request time, `Dispatcher.pickPlugins(from, to, overrides)` returns available plugins matching the conversion, sorted highest-priority first. If the top plugin errors, the next one is tried automatically — that's how Word failing over to LibreOffice works.

Templates in argv / URL / body support `{{input}}`, `{{output_ext}}`, `{{voice}}`, etc., with `|default:VALUE`, `|json`, `|shell` filters.

See [PLUGINS.md](PLUGINS.md) for authoring guide.

## Sinks (post-processing)

Sinks aren't in the plugin system — they're first-class Go code because they need to compose (a single PDF can go to Books AND Reading List AND get tagged). Sinks live in `internal/core/publish.go` and are invoked after every successful artifact.

Adding a new sink is straightforward: add the config fields, add the case in `applyPostProcessing`, write a small `publishToX` function that returns a `PublishResult{Target, OK, Detail, Error}`.

## Persistence

- `~/.mac-transformer/config.json` — user settings (recipes, watch folders, publish flags, API keys)
- `~/.mac-transformer/jobs.jsonl` — completed/failed/cancelled jobs, one JSON per line, capped at 5000
- `~/.mac-transformer/plugins/` — user-added plugin manifests
- `~/Library/Application Support/MacTransformer/plugins/` — bundled plugins unpacked at launch (so `osascript` can reach the .applescript files)

## Concurrency

- The queue runs jobs **one at a time**. Sequential execution prevents Word/PowerPoint racing over the same document library, and keeps Kokoro's TCP connection stable.
- Event subscribers (SSE stream, tray, notifier) are drained into per-subscriber channels; slow subscribers drop events rather than block the queue.
- Cancel uses `context.WithCancel` — subprocesses spawned via `exec.CommandContext` honor it.

## Startup

```
main() → run()
  ├── unpackBundledPlugins()          // embed → ~/Library/Application Support/…
  ├── core.LoadStore()                 // read config.json
  ├── plugin.Load()                    // scan + probe manifests
  ├── core.NewHistory()                // open jobs.jsonl
  ├── core.NewQueue()                  // hydrate from history
  ├── notify.Watcher.Run() goroutine   // subscribe → osascript notify
  ├── watch.Manager.Run() goroutine    // fsnotify → Queue.Submit
  ├── httpapi.Server.Start()           // bind :8771
  ├── wailsApp.Run()                   // GUI (blocks)
```

Shutdown is Wails-driven; the process is killed and `defer` closes clean up on their own.
