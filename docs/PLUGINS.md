# Writing plugins

A **plugin** is a converter — it takes one file format and produces another. Every plugin is a YAML file dropped into `plugins/` (bundled) or `~/.mac-transformer/plugins/` (user).

Post-processing **sinks** (Books, Music, Notion, etc.) are first-class Go code, not plugins — see [ARCHITECTURE.md](ARCHITECTURE.md).

## Minimum viable plugin

```yaml
# ~/.mac-transformer/plugins/my-tool.yaml
name: my-tool
description: What this plugin does
version: 1

capabilities:
  - from: [docx]
    to:   [md]
    priority: 40

requires:
  - kind: binary
    name: my-cli-tool

run:
  kind: command
  argv: [my-cli-tool, --input, "{{input}}", --output, "{{output}}"]
```

Save it, click **Plugins → Re-probe** in Settings (or `POST /plugins/reload`). Done. The dispatcher will use it whenever someone asks for `docx → md`.

## Schema reference

### Top-level fields

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | Unique identifier |
| `description` | string | no | Human-readable, shown in Settings |
| `version` | int | no | Bump when you change semantics |
| `capabilities` | list | yes | What conversions this plugin can do |
| `requires` | list | no | Availability probes; empty = always available |
| `run` | object | yes | How to invoke the plugin |

### `capabilities[]`

```yaml
- from: [docx, doc]        # source formats
  to:   [pdf]              # target formats
  priority: 100            # 0-100+; higher wins ties
```

Formats are lowercase extensions without the dot (`pdf`, `docx`, `md`, `adoc`).

### `requires[]`

Probes run **once at load time**. If any fails, the plugin is marked unavailable but the manifest is kept — the Settings view shows the reason.

```yaml
requires:
  # Executable on PATH
  - kind: binary
    name: pandoc

  # macOS app bundle installed
  - kind: app-bundle
    bundle_id: com.microsoft.Word

  # HTTP endpoint reachable
  - kind: http
    url: http://127.0.0.1:8770/health
    expect_status: 200

  # Ruby gem CLI on PATH
  - kind: gem
    name: asciidoctor-pdf

  # Absolute file exists
  - kind: file
    path: /Library/Fonts/Some.ttf
```

Users hit `POST /plugins/reload` (or click Re-probe) after installing new tools to refresh availability without restarting.

### `run` — command kind

```yaml
run:
  kind: command
  argv:
    - my-tool
    - "{{input}}"
    - -o
    - "{{output}}"
  env:
    MY_TOOL_OPT: "{{voice | default:standard}}"
  stdin: "{{input_text}}"           # optional — string piped to stdin
  workdir: "{{plugin_dir}}"         # optional — cwd for the process
  output_from: file                 # file (default) or stdout
  produces: "{{output_dir}}/{{input_stem}}.{{output_ext}}"
                                    # optional — where the tool ACTUALLY writes
                                    # (moved to {{output}} after)
  timeout_seconds: 180
```

- `argv[0]` is the executable; the rest are arguments. No shell — direct exec.
- `output_from: stdout` captures stdout and writes it to `{{output}}` — useful for tools that print to stdout.
- `produces:` handles tools that pick their own output path — the executor moves the result to `{{output}}`.

### `run` — http kind

```yaml
run:
  kind: http
  method: POST                      # default POST
  url: http://127.0.0.1:8770/tts
  headers:
    Content-Type: application/json
  body_json:                        # values are template-expanded
    text: "{{input_text}}"
    voice: "{{voice | default:af_bella}}"
    speed: "{{speed}}"              # coerced to number if key ends in "speed" or "_number"
    format: "{{output_ext}}"
  response_to_file: "{{output}}"    # streams response body to disk
  timeout_seconds: 3600
```

- If the response Content-Type is JSON, `response_to_file` still writes the raw body.
- Alternative: `body_template: "raw string with {{substitutions}}"` for non-JSON bodies.

### Template variables

Available in argv, env values, url, body_json, body_template, produces:

| Variable | Value |
|---|---|
| `{{input}}` | absolute path to the source file |
| `{{input_stem}}` | basename without extension |
| `{{input_ext}}` | source extension, lowercase, no dot |
| `{{input_text}}` | *file contents* (loaded automatically when a plugin references `input_text`) |
| `{{output}}` | absolute path the artifact should end up at |
| `{{output_stem}}`, `{{output_ext}}`, `{{output_dir}}` | components of `{{output}}` |
| `{{plugin_dir}}` | the plugin's manifest directory (companion scripts live here) |
| `{{voice}}` | TTS voice from the request |
| `{{speed}}` | TTS speed multiplier |

### Filters

Chain with `|`:

```yaml
"{{voice | default:af_bella}}"       # fall back if unset
"{{input | shell}}"                  # single-quote for shell safety
"{{input | json}}"                   # JSON-encode as a string literal
"{{input_stem | trim}}"              # strip surrounding whitespace
```

## Priority rules

Given multiple plugins that can perform `from → to`:

1. **Explicit override wins.** `request.plugin_overrides["docx->pdf"] = "libreoffice-headless"` skips others.
2. **Priority sort.** Higher number wins.
3. **Fallback on failure.** If the top-priority plugin errors out (subprocess crash, HTTP 500, timeout), the next one is tried automatically — the dispatcher records the plugin actually used in the step's `Chain`.

Recommended priority bands:

- 100 — native app (Word, PowerPoint, Kokoro) — highest fidelity for that source
- 60–80 — best-in-class general tool (asciidoctor-pdf for .adoc, pdftotext for pdf, ocr-pdf for scanned pdfs)
- 30–50 — generic tool (Pandoc, LibreOffice) — works for everything but not always highest quality
- 100 — passthrough identity (foo → foo — always available, so the pipeline works when the user asks for the same format)

## Companion scripts

Plugins can ship helper scripts alongside their YAML:

```
~/.mac-transformer/plugins/
  my-tool.yaml
  my-tool.applescript
  my-tool-wrapper.sh
```

Reference them via `{{plugin_dir}}/…` in argv. Shell scripts should be `chmod +x`.

## Real examples

See the bundled plugins in [`plugins/`](../plugins) for working examples:

- `libreoffice-headless.yaml` — a command plugin with `produces:` handling
- `kokoro-tts.yaml` — an HTTP plugin with `body_json`
- `microsoft-word-macos.yaml` + `word-convert.applescript` — command plugin with a companion script
- `ocr-pdf.yaml` + `pdf-text-smart.sh` + `ocr-pdf.swift` — command plugin with fallback logic in a wrapper script

## Debugging

- Server log (stderr from `mac-transformer` binary or the .app's Console log) shows plugin selection and errors.
- `GET /plugins` returns availability + reason for anything that failed a probe.
- To force a specific plugin: pass `plugin_overrides` in the request.
