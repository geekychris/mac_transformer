package core

import (
	"time"
)

// Status is the lifecycle state of a Job.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// TTSOptions selects the speech engine and its parameters.
type TTSOptions struct {
	Engine string  `json:"engine"` // "kokoro" | "say"
	Voice  string  `json:"voice,omitempty"`
	Speed  float64 `json:"speed,omitempty"`
}

// OutputLayout picks where artifacts land relative to OutputDir / the input.
//   - "flat":      OutputDir/<stem>.<ext>          (default — matches historical behavior)
//   - "subfolder": OutputDir/<stem>/<stem>.<ext>   (one folder per input, easy to move together)
//   - "beside":    <dir(input)>/<stem>.<ext>       (right next to the source; OutputDir ignored)
type OutputLayout string

const (
	LayoutFlat      OutputLayout = "flat"
	LayoutSubfolder OutputLayout = "subfolder"
	LayoutBeside    OutputLayout = "beside"
)

// Request is what the REST/MCP callers submit.
type Request struct {
	InputPath       string            `json:"path"`
	Outputs         []OutputKind      `json:"outputs"`
	TTS             TTSOptions        `json:"tts,omitempty"`
	OutputDir       string            `json:"output_dir,omitempty"`
	Layout          OutputLayout      `json:"layout,omitempty"`
	PluginOverrides map[string]string `json:"plugin_overrides,omitempty"` // "docx->pdf" -> plugin name
	Recipe          string            `json:"recipe,omitempty"`           // name of a saved recipe to apply
	Force           bool              `json:"force,omitempty"`            // bypass duplicate detection

	// Publish overrides. *bool so "not set" is distinguishable from "false"
	// (== respect the config default) vs "true"/"false" (== force override).
	// Split audio into per-chapter files (detected from headings) with
	// ID3 tags. Only meaningful when outputs includes "audio".
	AudioByChapter *bool `json:"audio_by_chapter,omitempty"`

	// PDF metadata stamped onto produced PDFs. If empty, we derive Title
	// from the input filename and default Author to "Mac Transformer".
	PDFTitle   string `json:"pdf_title,omitempty"`
	PDFAuthor  string `json:"pdf_author,omitempty"`
	PDFSubject string `json:"pdf_subject,omitempty"`

	PublishToBooks       *bool    `json:"publish_to_books,omitempty"`
	BooksCollection      string   `json:"books_collection,omitempty"`
	PublishToMusic       *bool    `json:"publish_to_music,omitempty"`
	MusicPlaylist        string   `json:"music_playlist,omitempty"`
	MusicGenre           string   `json:"music_genre,omitempty"`
	PublishToReadingList *bool    `json:"publish_to_reading_list,omitempty"`
	PublishToObsidian    *bool    `json:"publish_to_obsidian,omitempty"`
	PublishToNotion      *bool    `json:"publish_to_notion,omitempty"`
	Tags                 []string `json:"tags,omitempty"`
}

// Artifact is one produced file (plus how it was produced).
type Artifact struct {
	Kind    OutputKind      `json:"kind"`
	Format  Format          `json:"format"`
	Path    string          `json:"path"`
	Plugin  string          `json:"plugin"`
	Bytes   int64           `json:"bytes"`
	Publish []PublishResult `json:"publish,omitempty"`
	Tags    []string        `json:"tags,omitempty"`
}

// PublishResult records a single sink action (e.g. "added to Books",
// "tagged with #Research"). Failures don't fail the job — the artifact
// still exists on disk — but they're surfaced in the queue view so the
// user knows to retry manually.
type PublishResult struct {
	Target string `json:"target"` // "books" | "music" | "tags"
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
}

// StepStatus is the per-step lifecycle within a running job. It goes
// pending -> running -> completed / failed. Populated up-front so the
// frontend can render "3 pending, currently on step 2" without having to
// guess how many artifacts are still coming.
type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepCompleted StepStatus = "completed"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

// StepReport records what the dispatcher did for a single output. It's
// populated in-place as the step transitions — Status flips first, Plugin/
// Chain fill in as the dispatcher picks candidates, Ended/Error land last.
type StepReport struct {
	Kind    OutputKind `json:"kind"`
	Status  StepStatus `json:"status"`
	Plugin  string     `json:"plugin,omitempty"`
	Chain   []string   `json:"chain,omitempty"` // input -> intermediate -> output plugin names
	Message string     `json:"message,omitempty"` // human-readable progress hint (e.g. "extracting text")
	Error   string     `json:"error,omitempty"`
	Started time.Time  `json:"started,omitempty"`
	Ended   time.Time  `json:"ended,omitempty"`
}

// Job is the tracked lifecycle of one Request.
type Job struct {
	ID        string       `json:"id"`
	Request   Request      `json:"request"`
	Status    Status       `json:"status"`
	Message   string       `json:"message,omitempty"`
	Artifacts []Artifact   `json:"artifacts,omitempty"`
	Steps     []StepReport `json:"steps,omitempty"`
	InputHash string       `json:"input_hash,omitempty"` // sha256 of source file, for dedup
	Created   time.Time    `json:"created"`
	Started   time.Time    `json:"started,omitempty"`
	Ended     time.Time    `json:"ended,omitempty"`
}
