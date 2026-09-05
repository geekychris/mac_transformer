package core

import "testing"

func TestDetectChapters_Markdown(t *testing.T) {
	text := `# Intro

Some intro text.

# Body

Body content here.

# Conclusion

Wrap-up.`
	chs := DetectChapters(text, "md")
	if len(chs) != 3 {
		t.Fatalf("want 3 chapters, got %d", len(chs))
	}
	if chs[0].Title != "Intro" || chs[1].Title != "Body" || chs[2].Title != "Conclusion" {
		t.Errorf("titles wrong: %+v", chs)
	}
	// Bodies should NOT include the heading line.
	if !contains(chs[0].Body, "Some intro text") {
		t.Errorf("body[0] missing content: %q", chs[0].Body)
	}
}

func TestDetectChapters_AsciiDoc(t *testing.T) {
	text := `= Book Title

= Prologue

Setup.

= Chapter One

Content.`
	chs := DetectChapters(text, "adoc")
	if len(chs) < 2 {
		t.Fatalf("want ≥2 chapters, got %d", len(chs))
	}
}

func TestDetectChapters_NoHeadings(t *testing.T) {
	text := "just a paragraph\n\nand another paragraph"
	chs := DetectChapters(text, "txt")
	if len(chs) != 1 {
		t.Fatalf("want 1 fallback chapter, got %d", len(chs))
	}
	if chs[0].Body == "" {
		t.Errorf("fallback chapter body empty")
	}
}

func TestDetectChapters_ChapterWord(t *testing.T) {
	text := `Chapter 1

First body.

Chapter 2: The Beginning

Second body.`
	chs := DetectChapters(text, "txt")
	if len(chs) < 2 {
		t.Fatalf("want ≥2 chapters, got %d: %+v", len(chs), chs)
	}
}

func contains(hay, needle string) bool {
	return len(hay) >= len(needle) && indexOf(hay, needle) >= 0
}

func indexOf(hay, needle string) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
