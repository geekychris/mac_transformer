package core

import "testing"

func TestDetectFormat(t *testing.T) {
	cases := []struct {
		path string
		want Format
	}{
		{"/tmp/foo.docx", FormatDOCX},
		{"/tmp/foo.PPTX", FormatPPTX},
		{"foo.pdf", FormatPDF},
		{"foo.md", FormatMD},
		{"foo.markdown", "md"}, // aliased
		{"foo.adoc", FormatADOC},
		{"foo.asciidoc", "adoc"},
		{"foo.htm", "html"},
		{"foo.text", "txt"},
		{"noext", ""}, // unknown = empty
	}
	for _, c := range cases {
		got := DetectFormat(c.path)
		if got != c.want {
			t.Errorf("DetectFormat(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestDesiredFormat(t *testing.T) {
	cases := []struct {
		kind   OutputKind
		engine string
		want   Format
	}{
		{OutputPDF, "", FormatPDF},
		{OutputText, "", FormatTXT},
		{OutputAudio, "kokoro", FormatMP3},
		{OutputAudio, "say", FormatAIFF},
		{OutputSummary, "", "md"},
		{"unknown", "", ""},
	}
	for _, c := range cases {
		got := DesiredFormat(c.kind, c.engine)
		if got != c.want {
			t.Errorf("DesiredFormat(%q, %q) = %q, want %q", c.kind, c.engine, got, c.want)
		}
	}
}
