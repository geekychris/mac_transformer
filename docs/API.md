# API reference

Mac Transformer runs a REST server on `127.0.0.1:8771` and an MCP JSON-RPC endpoint on the same port.

## REST

All endpoints accept and return JSON. CORS is open to any origin (server binds to localhost only, so this is safe).

### Health

```
GET /health
```

```json
{
  "ok": true,
  "version": "0.1.0",
  "plugins": 13,
  "port": 8771,
  "queue_pending": 0,
  "queue_running": 0,
  "queue_completed": 47,
  "queue_failed": 3
}
```

### Submit a job

```
POST /transform
Content-Type: application/json
```

Body (all fields optional except `path`):

```json
{
  "path": "/absolute/path/to/document.docx",
  "outputs": ["pdf", "text", "audio", "summary"],
  "tts": { "engine": "kokoro", "voice": "af_bella", "speed": 1.2 },
  "output_dir": "/tmp/out",
  "layout": "beside",
  "audio_by_chapter": true,
  "publish_to_books": true,
  "books_collection": "Research",
  "publish_to_music": true,
  "music_playlist": "Read later",
  "music_genre": "Spoken",
  "publish_to_reading_list": true,
  "publish_to_obsidian": true,
  "publish_to_notion": false,
  "tags": ["Follow-up", "2026"],
  "pdf_title": "My Nice Title",
  "pdf_author": "Chris",
  "pdf_subject": "…",
  "plugin_overrides": { "docx->pdf": "libreoffice-headless" },
  "recipe": "Audiobook",
  "force": false
}
```

Returns:

- **202 Accepted** with the Job JSON (queued) — normal case
- **200 OK** with a previous Job JSON, and `X-Duplicate-Of: <job_id>` header — same content already processed. Retry with `"force": true` to re-run
- **400 Bad Request** on validation errors (non-absolute path, missing file, invalid recipe name, …)

### Poll a specific job

```
GET /jobs/{id}
```

Returns the full Job with `steps[]`, `artifacts[]`, `status`, etc.

### List all jobs

```
GET /jobs
```

### Download an artifact

```
GET /jobs/{id}/artifacts/{name}
```

### Open / reveal an artifact

```
POST /open        {"path": "..."}
POST /reveal      {"path": "..."}
```

- `/open` — hands the path to macOS `open` (Launch Services picks the default handler)
- `/reveal` — `open -R` (show in Finder without opening)

Both **validate** the path is an artifact of a known job — 400 otherwise. Prevents this endpoint from being used to open arbitrary files by anyone with LAN access.

### Cancel a job

```
POST /jobs/{id}/cancel
```

- **200 OK** with updated Job — cancelled
- **404** — no such job
- **409** — already finished

### Retry a failed job

```
POST /jobs/{id}/retry
Content-Type: application/json

{"kinds": ["audio"]}   // optional — defaults to all non-completed steps
```

Creates a NEW job whose Outputs = the specified kinds. The original job is preserved. Response includes header `X-Retry-Of: <original_id>`.

### Publish retroactively

```
POST /jobs/{id}/publish
Content-Type: application/json

{
  "publish_to_books": true,
  "books_collection": "Research",
  "publish_to_music": true,
  "music_playlist": "Podcasts",
  "publish_to_reading_list": true,
  "publish_to_obsidian": true,
  "publish_to_notion": false,
  "tags": ["Retro"]
}
```

Applies the given sinks to every applicable artifact of the job (PDFs to Books, audio to Music, etc.). Prior publish results for a target you re-request are replaced. Returns the updated Job.

### List plugins

```
GET /plugins
```

Each plugin has `name`, `description`, `capabilities`, `available`, `reason` (when unavailable), `bundled`.

### Re-probe plugins

```
POST /plugins/reload
```

Re-runs availability probes without re-reading manifests. Fast — call after installing a new tool.

### Voices

```
GET /voices?engine=kokoro
GET /voices?engine=say
```

Returns `{ "engine": "...", "voices": ["..."] }`.

### Config

```
GET /config
PUT /config
```

The full config JSON. See [CONFIG.md](CONFIG.md).

### Recipes

```
GET /recipes
```

Returns the saved recipes array. Recipes are stored in Config; edit via `PUT /config`.

### Search

```
GET /search?q=your+query
```

Case-insensitive substring search over every completed job's `.txt`/`.md` artifacts. Returns up to 50 hits:

```json
[
  {
    "job_id": "j_abc",
    "path": "/…/notes.txt",
    "snippet": "…80 chars either side of the first match…"
  }
]
```

### Event stream (SSE)

```
GET /events
Accept: text/event-stream
```

Emits `created`, `started`, `progress`, `completed`, `failed`, `cancelled` events. Each `data:` line is a JSON payload:

```
event: progress
data: {"type":"progress","job":{...}}
```

Every state transition emits a `progress`. Terminal states also emit their specific type.

## MCP

```
POST /mcp
Content-Type: application/json
```

JSON-RPC 2.0 requests. Supported methods:

- `initialize`
- `tools/list`
- `tools/call`
- `ping`

Tools:

| Name | Description |
|---|---|
| `transform` | Submit a job, block until finished, return artifacts |
| `publish_job` | Retroactively apply publishing sinks to an existing job |
| `list_plugins` | Enumerate loaded plugins + availability |
| `list_voices` | Voice list for `kokoro` or `say` |

Register the endpoint with Claude Code:

```sh
claude mcp add --transport http mac-transformer http://127.0.0.1:8771/mcp
```

After a Claude Code restart, tools appear as `mcp__mac-transformer__transform`, etc.

## CLI (`mt`)

Not an HTTP endpoint but a thin client over the REST API.

```sh
mt transform FILE [FILE...] [flags]
mt jobs
mt plugins
mt watch                                          # SSE stream to stdout
```

Flags mirror the REST fields. See `mt --help` for the full list.
