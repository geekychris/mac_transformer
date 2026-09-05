package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Config is user-editable settings persisted to ~/.mac-transformer/config.json.
// Defaults are chosen so a fresh install works without any config file.
type Config struct {
	// Default engine when a Request omits tts.engine.
	DefaultTTSEngine string `json:"default_tts_engine"` // "kokoro" | "say"
	// Default voice for Kokoro.
	DefaultKokoroVoice string `json:"default_kokoro_voice"`
	// Default voice for macOS `say`.
	DefaultSayVoice string `json:"default_say_voice"`
	// Default speed for TTS (0.5..2.0).
	DefaultSpeed float64 `json:"default_speed"`

	// Where artifacts land when Request.OutputDir is empty.
	OutputDir string `json:"output_dir"`

	// Default layout when Request.Layout is unset. See OutputLayout.
	DefaultLayout OutputLayout `json:"default_layout"`

	// Publish to Apple Books after producing a PDF.
	PublishToBooks         bool   `json:"publish_to_books"`
	DefaultBooksCollection string `json:"default_books_collection"`

	// Split audio into per-chapter files (from source headings) by default.
	AudioByChapter bool `json:"audio_by_chapter"`

	// Default author stamped onto every produced PDF's Info dict. Blank
	// falls back to "Mac Transformer" at write time.
	DefaultPDFAuthor string `json:"default_pdf_author,omitempty"`

	// Summarization endpoint (OpenAI-compatible chat completions API).
	// Works with Anthropic (via a proxy), OpenAI directly, or a local
	// Ollama server at http://127.0.0.1:11434/v1.
	SummarizerURL   string `json:"summarizer_url,omitempty"`
	SummarizerModel string `json:"summarizer_model,omitempty"`
	SummarizerKey   string `json:"summarizer_key,omitempty"` // Authorization: Bearer <key>
	SummarizerPrompt string `json:"summarizer_prompt,omitempty"`

	// Publish to Apple Music after producing an audio artifact.
	PublishToMusic       bool   `json:"publish_to_music"`
	DefaultMusicPlaylist string `json:"default_music_playlist"`
	DefaultMusicGenre    string `json:"default_music_genre"`

	// macOS Finder tags to apply to every produced artifact. Comma-separated
	// on the wire; stored as a list here so the UI can show a chip picker.
	DefaultTags []string `json:"default_tags,omitempty"`

	// Post a banner when a job finishes / fails / is cancelled.
	NotificationsEnabled bool `json:"notifications_enabled"`

	// Publish PDF artifacts to Safari's Reading List (iCloud-synced).
	PublishToReadingList bool `json:"publish_to_reading_list"`

	// Obsidian: copy markdown artifacts (summary, or extracted .md) into
	// the given vault directory. Empty vault path = disabled.
	PublishToObsidian bool   `json:"publish_to_obsidian"`
	ObsidianVault     string `json:"obsidian_vault,omitempty"`
	ObsidianFolder    string `json:"obsidian_folder,omitempty"` // subfolder within vault; default = ""

	// Notion: create pages in a database. Needs an integration token +
	// the target database id (32-char hex without dashes, as Notion's
	// URL uses).
	PublishToNotion bool   `json:"publish_to_notion"`
	NotionToken     string `json:"notion_token,omitempty"`
	NotionDatabase  string `json:"notion_database,omitempty"`

	// Named request templates. Users pick one from the DropZone / tray to
	// pre-fill outputs, TTS options, publish flags, and tags in one click.
	Recipes []Recipe `json:"recipes,omitempty"`

	// Directories to watch for new files. Each Watch fires a job on any
	// new-or-modified matching file using the specified recipe.
	WatchFolders []WatchFolder `json:"watch_folders,omitempty"`

	// User plugin directory. Bundled plugins are always loaded from the app
	// bundle; the user dir can override or add.
	UserPluginDir string `json:"user_plugin_dir"`

	// Port the local REST/MCP server binds to.
	Port int `json:"port"`

	// Override the resolved plugin for a specific conversion (e.g. "docx->pdf": "libreoffice-headless").
	PluginOverrides map[string]string `json:"plugin_overrides,omitempty"`
}

func Defaults() Config {
	home, _ := os.UserHomeDir()
	return Config{
		DefaultTTSEngine:   "kokoro",
		DefaultKokoroVoice: "af_bella",
		DefaultSayVoice:    "Samantha",
		DefaultSpeed:       1.0,
		OutputDir:          filepath.Join(home, "Documents", "MacTransformer"),
		DefaultLayout: LayoutBeside,
		UserPluginDir: filepath.Join(home, ".mac-transformer", "plugins"),
		Port:          8771,
		// Off by default — user opts in via Settings so we don't spam a
		// permission banner on first launch.
		NotificationsEnabled: false,
	}
}

// ConfigPath is the canonical location of the settings file.
func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mac-transformer", "config.json")
}

// Store holds the current config with mutex-protected mutation. Wails
// bindings and REST handlers both call through this.
type Store struct {
	mu   sync.RWMutex
	cfg  Config
	path string
}

func LoadStore() (*Store, error) {
	s := &Store{path: ConfigPath(), cfg: Defaults()}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	var override Config
	if err := json.Unmarshal(data, &override); err != nil {
		return s, err
	}
	s.cfg = merge(s.cfg, override)
	return s, nil
}

func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *Store) Set(next Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = merge(Defaults(), next)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}

// merge fills zero fields of override from base. Explicit user zero values
// (e.g. Speed = 0) are treated as "unset" — acceptable for our config since
// no field has a meaningful zero.
func merge(base, override Config) Config {
	out := base
	if override.DefaultTTSEngine != "" {
		out.DefaultTTSEngine = override.DefaultTTSEngine
	}
	if override.DefaultKokoroVoice != "" {
		out.DefaultKokoroVoice = override.DefaultKokoroVoice
	}
	if override.DefaultSayVoice != "" {
		out.DefaultSayVoice = override.DefaultSayVoice
	}
	if override.DefaultSpeed != 0 {
		out.DefaultSpeed = override.DefaultSpeed
	}
	if override.OutputDir != "" {
		out.OutputDir = override.OutputDir
	}
	if override.DefaultLayout != "" {
		out.DefaultLayout = override.DefaultLayout
	}
	// Booleans get copied unconditionally — the merge model here is "user
	// file wins, defaults fill gaps", and users may legitimately want to
	// set publish_to_books=false explicitly.
	out.PublishToBooks = override.PublishToBooks
	out.PublishToMusic = override.PublishToMusic
	out.AudioByChapter = override.AudioByChapter
	if override.DefaultPDFAuthor != "" {
		out.DefaultPDFAuthor = override.DefaultPDFAuthor
	}
	if override.SummarizerURL != "" {
		out.SummarizerURL = override.SummarizerURL
	}
	if override.SummarizerModel != "" {
		out.SummarizerModel = override.SummarizerModel
	}
	if override.SummarizerKey != "" {
		out.SummarizerKey = override.SummarizerKey
	}
	if override.SummarizerPrompt != "" {
		out.SummarizerPrompt = override.SummarizerPrompt
	}
	out.NotificationsEnabled = override.NotificationsEnabled
	out.PublishToReadingList = override.PublishToReadingList
	out.PublishToObsidian = override.PublishToObsidian
	if override.ObsidianVault != "" {
		out.ObsidianVault = override.ObsidianVault
	}
	if override.ObsidianFolder != "" {
		out.ObsidianFolder = override.ObsidianFolder
	}
	out.PublishToNotion = override.PublishToNotion
	if override.NotionToken != "" {
		out.NotionToken = override.NotionToken
	}
	if override.NotionDatabase != "" {
		out.NotionDatabase = override.NotionDatabase
	}
	if override.DefaultBooksCollection != "" {
		out.DefaultBooksCollection = override.DefaultBooksCollection
	}
	if override.DefaultMusicPlaylist != "" {
		out.DefaultMusicPlaylist = override.DefaultMusicPlaylist
	}
	if override.DefaultMusicGenre != "" {
		out.DefaultMusicGenre = override.DefaultMusicGenre
	}
	if override.DefaultTags != nil {
		out.DefaultTags = override.DefaultTags
	}
	if override.Recipes != nil {
		out.Recipes = override.Recipes
	}
	if override.WatchFolders != nil {
		out.WatchFolders = override.WatchFolders
	}
	if override.UserPluginDir != "" {
		out.UserPluginDir = override.UserPluginDir
	}
	if override.Port != 0 {
		out.Port = override.Port
	}
	if override.PluginOverrides != nil {
		out.PluginOverrides = override.PluginOverrides
	}
	return out
}
