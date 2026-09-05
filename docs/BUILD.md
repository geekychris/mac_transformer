# Building from source

## Prerequisites (build)

- **Go** 1.23 or newer
- **Node.js** 20 or newer + npm (for the Wails frontend)
- **Wails v3 CLI**: `go install github.com/wailsapp/wails/v3/cmd/wails3@latest`
- **Xcode Command Line Tools** (`xcode-select --install`) — provides `swift`, `swiftc`, `codesign`

## Prerequisites (runtime, optional)

You need any subset of these — the app detects what's installed at launch and marks plugins available accordingly.

| Tool | Purpose | Install |
|---|---|---|
| **LibreOffice** | Office → PDF/txt | `brew install --cask libreoffice` |
| **Pandoc** | Markdown/AsciiDoc/HTML → PDF/txt | `brew install pandoc` |
| **BasicTeX** or **MacTeX** | Pandoc's PDF pipeline | `brew install --cask basictex` (~100MB) |
| **Poppler** | `pdftotext` for PDF text extraction | `brew install poppler` |
| **asciidoctor-pdf** | Best-in-class .adoc → PDF | `gem install asciidoctor-pdf` |
| **ffmpeg** | Audio ID3 tagging + cover art embedding | `brew install ffmpeg` |
| **Microsoft Word / PowerPoint** | Highest-fidelity Office → PDF | Microsoft 365 |
| **Kokoro server** | Local neural TTS | See [kokoro_runtime](https://github.com/geekychris/kokoro_runtime) |

The `scripts/install-deps.sh` script prompts before installing each of the brew-available ones. Everything else the app just detects and warns if missing.

## Quick build

```sh
./scripts/install.sh
```

This does:

1. Installs `wails3` CLI if missing
2. Generates Wails TypeScript bindings from Go exported services
3. Builds the React frontend (`vite build`)
4. Runs `wails3 package` which builds the Go binary + assembles the `.app` bundle
5. Ad-hoc code signs it
6. Copies to `/Applications/Mac Transformer.app`
7. Registers with Launch Services (for Finder Open With)
8. Installs the Automator Quick Action to `~/Library/Services/`
9. Optionally runs `claude mcp add` if the Claude Code CLI is present

Environment knobs:

- `MT_BUILD=0` — reuse the existing binary
- `MT_INSTALL_QUICK_ACTION=0` — skip the Finder Quick Action
- `MT_INSTALL_MCP=0` — skip `claude mcp add`
- `MT_INSTALL_TO_APPLICATIONS=0` — keep the `.app` in `./bin` instead of `/Applications`

## Manual build

```sh
# Frontend
cd frontend && npm install && npm run build && cd ..

# Wails bindings
wails3 generate bindings

# Debug binary
go build -o bin/mac-transformer .

# Or full .app bundle
wails3 package
```

The `.app` bundle lands at `bin/mac-transformer.app`.

## Dev loop

```sh
wails3 dev
```

This runs Vite's dev server and rebuilds Go on save — hot reload for both sides.

For the REST server alone (no GUI), use the headless variant:

```sh
go run ./cmd/headless -plugins ./plugins
```

## Running tests

```sh
go test ./...

# With coverage
go test -cover ./internal/...
```

Some tests (Finder tag xattr) need macOS-specific tooling and won't run on other platforms.

## Building the CLI

```sh
go build -o bin/mt ./cmd/mt
```

Add `bin/` to `PATH` (or symlink `bin/mt` into `/usr/local/bin`).

## Building a signed release

Ad-hoc signing (what `install.sh` does by default) works for personal use but macOS may quarantine downloaded copies. For distribution:

1. Enroll in the Apple Developer Program
2. Create a Developer ID certificate
3. Configure Wails: `wails3 signing`
4. Build: `wails3 package` — will sign with your certificate
5. Notarize: `wails3 sign --notarize`

See `wails3 docs` for details.

## Cross-platform

Currently macOS-only. The plugin system, dispatcher, publish sinks, and REST/MCP servers are all portable Go. Blockers for Linux/Windows:

- Finder Quick Action (macOS-specific — Automator workflow)
- CFBundleDocumentTypes (macOS-specific — Info.plist)
- Books.app, Music.app, Safari sinks (macOS-specific)
- osascript-based Word/PowerPoint plugins (would need Windows COM automation equivalents)
- macOS Vision OCR (would need Tesseract fallback)

The core (converter chains, format detection, REST/MCP, CLI) already runs on Linux — the `mt` CLI works against any running server, including one launched via `cmd/headless` on a Linux host.
