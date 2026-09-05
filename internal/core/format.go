package core

import (
	"path/filepath"
	"strings"
)

// Format is a lowercase file-type identifier without a leading dot ("pdf",
// "docx"). Plugins declare capabilities in terms of these strings.
type Format string

const (
	FormatPDF   Format = "pdf"
	FormatTXT   Format = "txt"
	FormatWAV   Format = "wav"
	FormatMP3   Format = "mp3"
	FormatAIFF  Format = "aiff"
	FormatDOCX  Format = "docx"
	FormatDOC   Format = "doc"
	FormatPPTX  Format = "pptx"
	FormatPPT   Format = "ppt"
	FormatRTF   Format = "rtf"
	FormatMD    Format = "md"
	FormatADOC  Format = "adoc"
)

// DetectFormat maps a file path to a Format via its extension. Returns empty
// string if the extension is unknown; callers should treat that as
// "unsupported input".
func DetectFormat(path string) Format {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	// Canonicalise a few aliases so plugins can just declare "md" / "adoc".
	switch ext {
	case "markdown":
		return "md"
	case "asciidoc":
		return "adoc"
	case "text":
		return "txt"
	case "htm":
		return "html"
	}
	return Format(ext)
}

// OutputKind is the high-level artifact the user requested. The dispatcher
// picks a Format for it based on the input and settings.
type OutputKind string

const (
	OutputPDF     OutputKind = "pdf"
	OutputText    OutputKind = "text"
	OutputAudio   OutputKind = "audio"
	OutputSummary OutputKind = "summary"
)

// DesiredFormat maps a high-level OutputKind to a concrete Format the
// dispatcher will ask plugins to produce.
func DesiredFormat(kind OutputKind, ttsEngine string) Format {
	switch kind {
	case OutputPDF:
		return FormatPDF
	case OutputText:
		return FormatTXT
	case OutputAudio:
		// `say` writes AIFF natively; Kokoro can produce either but MP3 is
		// smaller. The pipeline may post-convert.
		if ttsEngine == "say" {
			return FormatAIFF
		}
		return FormatMP3
	case OutputSummary:
		return "md"
	}
	return ""
}
