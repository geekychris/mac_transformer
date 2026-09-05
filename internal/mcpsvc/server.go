// Package mcpsvc implements a minimal Model Context Protocol server over a
// single POST endpoint (Streamable HTTP transport). Modelled after
// kokoro_runtime/crates/kokoro-server/src/mcp.rs for stylistic consistency.
//
// Exposes tools:
//   - transform: submit a conversion job (blocking until done)
//   - list_plugins: enumerate loaded plugins with availability status
//   - list_voices: enumerate voices for the selected TTS engine
package mcpsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/geekychris/mac_transformer/internal/core"
	"github.com/geekychris/mac_transformer/internal/plugin"
)

const (
	ProtocolVersion = "2025-06-18"
	ServerName      = "mac-transformer"
	ServerVersion   = "0.1.0"
)

type Handler struct {
	Queue    *core.Queue
	Registry *plugin.Registry
	Store    *core.Store
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "MCP endpoint requires POST", http.StatusMethodNotAllowed)
		return
	}
	var msg map[string]any
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		writeError(w, nil, -32700, "parse error: "+err.Error())
		return
	}
	id := msg["id"]
	method, _ := msg["method"].(string)
	params, _ := msg["params"].(map[string]any)

	// Notifications have no id — accept and drop.
	if id == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	var result any
	var errRPC *rpcError
	switch method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": ServerName, "version": ServerVersion},
		}
	case "tools/list":
		result = map[string]any{"tools": h.toolsList()}
	case "tools/call":
		result, errRPC = h.toolsCall(r.Context(), params)
	case "ping":
		result = map[string]any{}
	default:
		errRPC = &rpcError{Code: -32601, Message: "method not found: " + method}
	}

	resp := map[string]any{"jsonrpc": "2.0", "id": id}
	if errRPC != nil {
		resp["error"] = map[string]any{"code": errRPC.Code, "message": errRPC.Message}
	} else {
		resp["result"] = result
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type rpcError struct {
	Code    int
	Message string
}

func (h *Handler) toolsList() []map[string]any {
	return []map[string]any{
		{
			"name":        "transform",
			"description": "Convert a document to PDF/text/audio via the pluggable pipeline. Blocks until finished. Returns artifact paths.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Absolute path to the input document (docx/pptx/pdf/adoc/md/txt/rtf/…).",
					},
					"outputs": map[string]any{
						"type":        "array",
						"description": "Requested outputs: any subset of [\"pdf\",\"text\",\"audio\"]. Defaults to [\"pdf\"].",
						"items":       map[string]any{"type": "string", "enum": []string{"pdf", "text", "audio"}},
					},
					"tts_engine": map[string]any{
						"type": "string", "enum": []string{"kokoro", "say"},
						"description": "TTS engine to use for audio output. Defaults to config.",
					},
					"voice": map[string]any{"type": "string", "description": "Voice name — kokoro voice or macOS `say` voice."},
					"speed": map[string]any{"type": "number", "description": "Speech rate 0.5–2.0.", "minimum": 0.5, "maximum": 2.0},
					"output_dir": map[string]any{"type": "string", "description": "Directory to write artifacts. Defaults to config."},
				},
				"required": []string{"path"},
			},
		},
		{
			"name":        "list_plugins",
			"description": "List loaded converter plugins with capabilities and availability status.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "publish_job",
			"description": "Retroactively publish an existing job's artifacts to Apple Books (PDFs), Apple Music (audio), and/or apply macOS Finder tags. Blocks until all requested sinks have run.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"job_id":            map[string]any{"type": "string", "description": "Job id returned by transform."},
					"publish_to_books":  map[string]any{"type": "boolean", "description": "Add PDFs to Books.app."},
					"books_collection":  map[string]any{"type": "string", "description": "Books collection name."},
					"publish_to_music":  map[string]any{"type": "boolean", "description": "Add audio artifacts to Music.app."},
					"music_playlist":    map[string]any{"type": "string", "description": "Music playlist name."},
					"music_genre":       map[string]any{"type": "string", "description": "Music genre tag."},
					"tags":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "macOS Finder tags to apply to every artifact."},
				},
				"required": []string{"job_id"},
			},
		},
		{
			"name":        "list_voices",
			"description": "List available voices for a TTS engine.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"engine": map[string]any{"type": "string", "enum": []string{"kokoro", "say"}},
				},
				"required": []string{"engine"},
			},
		},
	}
}

func (h *Handler) toolsCall(ctx context.Context, params map[string]any) (any, *rpcError) {
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)

	switch name {
	case "transform":
		return h.callTransform(ctx, args)
	case "publish_job":
		return h.callPublishJob(ctx, args)
	case "list_plugins":
		return h.callListPlugins()
	case "list_voices":
		return h.callListVoices(ctx, args)
	default:
		return nil, &rpcError{Code: -32602, Message: "unknown tool: " + name}
	}
}

func (h *Handler) callPublishJob(ctx context.Context, args map[string]any) (any, *rpcError) {
	id, _ := args["job_id"].(string)
	if id == "" {
		return toolError("`job_id` is required"), nil
	}
	job, ok := h.Queue.Get(id)
	if !ok {
		return toolError("no such job: " + id), nil
	}
	if len(job.Artifacts) == 0 {
		return toolError("job has no artifacts"), nil
	}
	po := core.PublishOptions{}
	if v, ok := args["publish_to_books"].(bool); ok {
		po.Books = v
	}
	if v, ok := args["books_collection"].(string); ok {
		po.BooksCollection = v
	}
	if v, ok := args["publish_to_music"].(bool); ok {
		po.Music = v
	}
	if v, ok := args["music_playlist"].(string); ok {
		po.MusicPlaylist = v
	}
	if v, ok := args["music_genre"].(string); ok {
		po.MusicGenre = v
	}
	if raw, ok := args["tags"].([]any); ok {
		for _, r := range raw {
			if s, ok := r.(string); ok {
				po.Tags = append(po.Tags, s)
			}
		}
	}
	if !po.Books && !po.Music && len(po.Tags) == 0 {
		return toolError("nothing to publish"), nil
	}

	// Find any bundled plugin so we know where AppleScript helpers live.
	scriptDir := ""
	for _, lp := range h.Registry.All() {
		if lp.Manifest.Bundled {
			scriptDir = lp.Manifest.Dir
			break
		}
	}

	for i := range job.Artifacts {
		core.ApplyPostProcessing(ctx, &job.Artifacts[i], po, scriptDir)
	}
	h.Queue.EmitProgress(job)
	return jobSummary(job), nil
}

func (h *Handler) callTransform(ctx context.Context, args map[string]any) (any, *rpcError) {
	path, _ := args["path"].(string)
	if path == "" {
		return toolError("`path` is required"), nil
	}
	req := core.Request{InputPath: path}
	if raw, ok := args["outputs"].([]any); ok {
		for _, r := range raw {
			if s, ok := r.(string); ok {
				req.Outputs = append(req.Outputs, core.OutputKind(s))
			}
		}
	}
	if len(req.Outputs) == 0 {
		req.Outputs = []core.OutputKind{core.OutputPDF}
	}
	if s, ok := args["tts_engine"].(string); ok {
		req.TTS.Engine = s
	}
	if s, ok := args["voice"].(string); ok {
		req.TTS.Voice = s
	}
	if f, ok := args["speed"].(float64); ok {
		req.TTS.Speed = f
	}
	if s, ok := args["output_dir"].(string); ok {
		req.OutputDir = s
	}

	job := h.Queue.Submit(req)
	// Poll until terminal state. MCP callers expect blocking semantics.
	deadline := time.Now().Add(30 * time.Minute)
	for {
		j, _ := h.Queue.Get(job.ID)
		if j == nil {
			return toolError("job vanished"), nil
		}
		if j.Status == core.StatusCompleted || j.Status == core.StatusFailed || j.Status == core.StatusCancelled {
			return jobSummary(j), nil
		}
		if time.Now().After(deadline) {
			return toolError("job timed out"), nil
		}
		select {
		case <-ctx.Done():
			return toolError("client cancelled"), nil
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func (h *Handler) callListPlugins() (any, *rpcError) {
	var lines []string
	for _, lp := range h.Registry.All() {
		status := "OK"
		if !lp.Available {
			status = "UNAVAILABLE: " + lp.Reason
		}
		caps := []string{}
		for _, c := range lp.Manifest.Capabilities {
			caps = append(caps, fmt.Sprintf("%s->%s@%d",
				strings.Join(c.From, "|"), strings.Join(c.To, "|"), c.Priority))
		}
		lines = append(lines, fmt.Sprintf("%s [%s]: %s", lp.Manifest.Name, status, strings.Join(caps, ", ")))
	}
	return toolText(strings.Join(lines, "\n")), nil
}

func (h *Handler) callListVoices(ctx context.Context, args map[string]any) (any, *rpcError) {
	engine, _ := args["engine"].(string)
	if engine == "" {
		return toolError("`engine` is required"), nil
	}
	// The concrete voice-fetching code lives in httpapi to keep it consolidated.
	// We inline a slim variant here to avoid a circular import.
	switch engine {
	case "kokoro":
		v, err := fetchKokoroVoices(ctx)
		if err != nil {
			return toolError("kokoro: " + err.Error()), nil
		}
		return toolText(strings.Join(v, ", ")), nil
	case "say":
		v, err := macSayVoices(ctx)
		if err != nil {
			return toolError("say: " + err.Error()), nil
		}
		return toolText(strings.Join(v, ", ")), nil
	default:
		return toolError("unknown engine: " + engine), nil
	}
}

func jobSummary(j *core.Job) any {
	var lines []string
	lines = append(lines, fmt.Sprintf("Job %s: %s", j.ID, j.Status))
	if j.Message != "" {
		lines = append(lines, "Message: "+j.Message)
	}
	for _, a := range j.Artifacts {
		lines = append(lines, fmt.Sprintf("  - %s (%s, %d bytes, via %s)", a.Path, a.Format, a.Bytes, a.Plugin))
	}
	for _, s := range j.Steps {
		if s.Error != "" {
			lines = append(lines, fmt.Sprintf("  ! %s failed: %s", s.Kind, s.Error))
		}
	}
	return toolText(strings.Join(lines, "\n"))
}

func toolText(s string) any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": s}},
	}
}

func toolError(s string) any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": s}},
		"isError": true,
	}
}

func writeError(w http.ResponseWriter, id any, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": msg},
	})
}
