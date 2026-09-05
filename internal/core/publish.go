package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PublishOptions is the resolved set of publish choices for one job. Empty
// strings mean "not requested". Exported so httpapi can build one directly
// from a retroactive publish request without going through a full Request.
type PublishOptions struct {
	Books           bool
	BooksCollection string
	Music           bool
	MusicPlaylist   string
	MusicGenre      string
	ReadingList     bool
	Obsidian        bool
	ObsidianVault   string
	ObsidianFolder  string
	Notion          bool
	NotionToken     string
	NotionDatabase  string
	Tags            []string
}

// Keep lowercase-alias so existing dispatcher call sites don't churn.
type publishOptions = PublishOptions

func resolvePublish(cfg Config, req *Request) publishOptions {
	po := publishOptions{
		Books:           cfg.PublishToBooks,
		BooksCollection: cfg.DefaultBooksCollection,
		Music:           cfg.PublishToMusic,
		MusicPlaylist:   cfg.DefaultMusicPlaylist,
		MusicGenre:      cfg.DefaultMusicGenre,
		ReadingList:     cfg.PublishToReadingList,
		Tags:            append([]string{}, cfg.DefaultTags...),
	}
	if req.PublishToBooks != nil {
		po.Books = *req.PublishToBooks
	}
	if req.BooksCollection != "" {
		po.BooksCollection = req.BooksCollection
	}
	if req.PublishToMusic != nil {
		po.Music = *req.PublishToMusic
	}
	if req.MusicPlaylist != "" {
		po.MusicPlaylist = req.MusicPlaylist
	}
	if req.MusicGenre != "" {
		po.MusicGenre = req.MusicGenre
	}
	if req.PublishToReadingList != nil {
		po.ReadingList = *req.PublishToReadingList
	}
	po.Obsidian = cfg.PublishToObsidian
	po.ObsidianVault = cfg.ObsidianVault
	po.ObsidianFolder = cfg.ObsidianFolder
	if req.PublishToObsidian != nil {
		po.Obsidian = *req.PublishToObsidian
	}
	po.Notion = cfg.PublishToNotion
	po.NotionToken = cfg.NotionToken
	po.NotionDatabase = cfg.NotionDatabase
	if req.PublishToNotion != nil {
		po.Notion = *req.PublishToNotion
	}
	if len(req.Tags) > 0 {
		po.Tags = append([]string{}, req.Tags...)
	}
	return po
}

// ApplyPostProcessing is the exported form of applyPostProcessing. Same
// semantics, callable from packages outside core (e.g. httpapi for the
// retroactive-publish endpoint). Before running, it drops any prior
// publish results for the targets we're about to touch, so re-invoking
// publish shows fresh state rather than an audit trail.
func ApplyPostProcessing(ctx context.Context, art *Artifact, po PublishOptions, pluginDir string) {
	// Drop prior results for any target we're about to try, so a second
	// publish call shows the new outcome rather than appending to the old.
	filtered := art.Publish[:0]
	for _, pr := range art.Publish {
		drop := (pr.Target == "tags" && len(po.Tags) > 0) ||
			(pr.Target == "books" && po.Books) ||
			(pr.Target == "music" && po.Music) ||
			(pr.Target == "reading_list" && po.ReadingList) ||
			(pr.Target == "obsidian" && po.Obsidian) ||
			(pr.Target == "notion" && po.Notion)
		if !drop {
			filtered = append(filtered, pr)
		}
	}
	art.Publish = filtered
	applyPostProcessing(ctx, art, po, pluginDir)
}

// applyPostProcessing runs any publish steps that apply to a produced
// artifact. Errors don't propagate up — the artifact is on disk, publish
// is a side effect. We just annotate the artifact so the queue view can
// show what succeeded / needs attention.
//
// pluginDir is where books-publish.applescript and friends live (same as
// bundled plugins).
func applyPostProcessing(ctx context.Context, art *Artifact, po publishOptions, pluginDir string) {
	if len(po.Tags) > 0 {
		if err := setFinderTags(art.Path, po.Tags); err != nil {
			art.Publish = append(art.Publish, PublishResult{
				Target: "tags", OK: false,
				Detail: strings.Join(po.Tags, ", "),
				Error:  err.Error(),
			})
		} else {
			art.Publish = append(art.Publish, PublishResult{
				Target: "tags", OK: true,
				Detail: strings.Join(po.Tags, ", "),
			})
			art.Tags = po.Tags
		}
	}

	if po.Books && art.Format == FormatPDF {
		art.Publish = append(art.Publish, publishToBooks(ctx, art.Path, po.BooksCollection, pluginDir))
	}

	if po.ReadingList && art.Format == FormatPDF {
		art.Publish = append(art.Publish, publishToReadingList(ctx, art.Path, pluginDir))
	}

	if po.Obsidian && (art.Format == "md" || art.Format == FormatTXT) {
		art.Publish = append(art.Publish, publishToObsidian(art.Path, po.ObsidianVault, po.ObsidianFolder, po.Tags))
	}

	if po.Notion && (art.Format == "md" || art.Format == FormatTXT) {
		art.Publish = append(art.Publish, publishToNotion(ctx, art.Path, po.NotionToken, po.NotionDatabase, po.Tags))
	}

	if po.Music && isAudioFormat(art.Format) {
		art.Publish = append(art.Publish, publishToMusic(ctx, art.Path, po.MusicGenre, po.MusicPlaylist, pluginDir))
	}
}

// publishToBooks hands the PDF to Books via Launch Services (`open -a Books`).
// That triggers Books' built-in importer without needing Automation
// permission. If a collection is requested, an AppleScript follow-up tries
// to place it there — that's the optional bit that may need automation
// consent from the user the first time.
func publishToBooks(ctx context.Context, path, collection, pluginDir string) PublishResult {
	detail := "Books library"
	if collection != "" {
		detail = "Books › " + collection
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "open", "-a", "Books", path).Run(); err != nil {
		return PublishResult{Target: "books", OK: false, Detail: detail, Error: err.Error()}
	}
	// Give Books a moment to ingest before the collection tweak runs.
	// Skip when no collection was requested — most users won't have
	// approved automation and this would time out.
	if collection != "" {
		script := filepath.Join(pluginDir, "books-publish-collection.applescript")
		collectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := exec.CommandContext(collectCtx, "osascript", script, path, collection).Run(); err != nil {
			return PublishResult{
				Target: "books", OK: true, Detail: detail + " (collection move deferred)",
				Error: "collection: " + err.Error(),
			}
		}
	}
	return PublishResult{Target: "books", OK: true, Detail: detail}
}

// publishToMusic uses `open -a Music` for the base import. If genre or
// playlist is requested, an AppleScript follow-up sets those (which does
// require Automation permission for Music.app).
func publishToMusic(ctx context.Context, path, genre, playlist, pluginDir string) PublishResult {
	detail := "Music library"
	if playlist != "" {
		detail = "Music › " + playlist
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "open", "-a", "Music", path).Run(); err != nil {
		return PublishResult{Target: "music", OK: false, Detail: detail, Error: err.Error()}
	}
	if genre != "" || playlist != "" {
		script := filepath.Join(pluginDir, "music-publish-metadata.applescript")
		metaCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := exec.CommandContext(metaCtx, "osascript", script, path, genre, playlist).Run(); err != nil {
			return PublishResult{
				Target: "music", OK: true, Detail: detail + " (metadata deferred)",
				Error: "metadata: " + err.Error(),
			}
		}
	}
	return PublishResult{Target: "music", OK: true, Detail: detail}
}

// publishToReadingList adds the PDF (as a file:// URL) to Safari's Reading
// List. Needs Automation permission for Safari the first time.
func publishToReadingList(ctx context.Context, path, pluginDir string) PublishResult {
	script := filepath.Join(pluginDir, "reading-list-publish.applescript")
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "osascript", script, path).CombinedOutput()
	if err != nil {
		return PublishResult{Target: "reading_list", OK: false, Detail: "Safari Reading List", Error: fmt.Sprintf("%s: %s", err, strings.TrimSpace(string(out)))}
	}
	return PublishResult{Target: "reading_list", OK: true, Detail: "Safari Reading List"}
}

// publishToObsidian copies the markdown artifact into an Obsidian vault
// with YAML frontmatter that carries the source path, tags, and creation
// time. Skips silently if the vault path isn't a directory.
func publishToObsidian(path, vault, folder string, tags []string) PublishResult {
	if vault == "" {
		return PublishResult{Target: "obsidian", OK: false, Detail: "no vault configured", Error: "obsidian_vault not set"}
	}
	if fi, err := os.Stat(vault); err != nil || !fi.IsDir() {
		return PublishResult{Target: "obsidian", OK: false, Detail: vault, Error: fmt.Sprintf("vault not a directory: %s", vault)}
	}
	destDir := filepath.Join(vault, folder)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return PublishResult{Target: "obsidian", OK: false, Detail: destDir, Error: err.Error()}
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return PublishResult{Target: "obsidian", OK: false, Detail: destDir, Error: err.Error()}
	}
	// Prepend frontmatter if not already present.
	if !bytes.HasPrefix(body, []byte("---\n")) {
		fm := buildFrontmatter(path, tags)
		body = append([]byte(fm), body...)
	}
	destPath := filepath.Join(destDir, filepath.Base(path))
	if err := os.WriteFile(destPath, body, 0o644); err != nil {
		return PublishResult{Target: "obsidian", OK: false, Detail: destPath, Error: err.Error()}
	}
	return PublishResult{Target: "obsidian", OK: true, Detail: destPath}
}

func buildFrontmatter(sourcePath string, tags []string) string {
	tagLines := ""
	if len(tags) > 0 {
		for _, t := range tags {
			tagLines += "  - " + t + "\n"
		}
	}
	if tagLines == "" {
		tagLines = "  - MacTransformer\n"
	}
	return fmt.Sprintf("---\nsource: %q\ncreated: %s\ntags:\n%s---\n\n",
		sourcePath, time.Now().Format(time.RFC3339), tagLines)
}

// publishToNotion creates a Notion page in the configured database, with
// the artifact's contents as the page body. Blocks are simple paragraphs;
// we don't try to preserve Markdown formatting beyond splitting by
// blank-line-separated paragraphs. Users can convert the block later.
func publishToNotion(ctx context.Context, path, token, dbID string, tags []string) PublishResult {
	if token == "" || dbID == "" {
		return PublishResult{Target: "notion", OK: false, Detail: "no token/db configured", Error: "notion_token or notion_database missing"}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return PublishResult{Target: "notion", OK: false, Error: err.Error()}
	}
	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	// Build children — one paragraph block per blank-line-separated chunk.
	// Notion caps at 100 blocks per request; we truncate.
	paragraphs := strings.Split(string(body), "\n\n")
	if len(paragraphs) > 90 {
		paragraphs = paragraphs[:90]
	}
	children := make([]map[string]any, 0, len(paragraphs))
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len(p) > 1900 {
			p = p[:1900] + "…"
		}
		children = append(children, map[string]any{
			"object": "block",
			"type":   "paragraph",
			"paragraph": map[string]any{
				"rich_text": []map[string]any{
					{"type": "text", "text": map[string]any{"content": p}},
				},
			},
		})
	}

	req := map[string]any{
		"parent": map[string]any{"database_id": dbID},
		"properties": map[string]any{
			// Notion's default title property is called "Name" but we
			// can't know the target db's schema without a lookup. Use
			// "title" which most templates alias.
			"Name": map[string]any{
				"title": []map[string]any{
					{"type": "text", "text": map[string]any{"content": title}},
				},
			},
		},
		"children": children,
	}
	buf, _ := json.Marshal(req)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.notion.com/v1/pages", bytes.NewReader(buf))
	if err != nil {
		return PublishResult{Target: "notion", OK: false, Error: err.Error()}
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Notion-Version", "2022-06-28")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return PublishResult{Target: "notion", OK: false, Error: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		return PublishResult{Target: "notion", OK: false, Detail: fmt.Sprintf("db %s", dbID[:8]), Error: fmt.Sprintf("%d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))}
	}
	return PublishResult{Target: "notion", OK: true, Detail: fmt.Sprintf("db %s", dbID[:8])}
}

func isAudioFormat(f Format) bool {
	return f == FormatMP3 || f == FormatAIFF || f == FormatWAV
}

// setFinderTags writes the macOS user-tags xattr on the file. XML plists
// work as well as binary; Finder and Spotlight accept either.
func setFinderTags(path string, tags []string) error {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><array>`)
	for _, t := range tags {
		b.WriteString("<string>")
		b.WriteString(xmlEscape(t))
		b.WriteString("</string>")
	}
	b.WriteString("</array></plist>")
	out, err := exec.Command("xattr", "-w", "com.apple.metadata:_kMDItemUserTags", b.String(), path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("xattr: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
