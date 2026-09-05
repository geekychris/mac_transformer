package core

import (
	"path/filepath"
	"testing"
)

func TestResolveArtifactPath_Flat(t *testing.T) {
	dir := t.TempDir()
	p, err := resolveArtifactPath("/src/foo.md", "foo", "pdf", dir, LayoutFlat)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "foo.pdf")
	if p != want {
		t.Errorf("got %q, want %q", p, want)
	}
}

func TestResolveArtifactPath_Subfolder(t *testing.T) {
	dir := t.TempDir()
	p, err := resolveArtifactPath("/src/foo.md", "foo", "pdf", dir, LayoutSubfolder)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "foo", "foo.pdf")
	if p != want {
		t.Errorf("got %q, want %q", p, want)
	}
}

func TestResolveArtifactPath_Beside(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "src", "foo.md")
	// Create the source dir so mkdirall doesn't blow up.
	_ = filepath.Join(dir, "src")
	p, err := resolveArtifactPath(inputPath, "foo", "pdf", "/ignored", LayoutBeside)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "src", "foo.pdf")
	if p != want {
		t.Errorf("got %q, want %q", p, want)
	}
}

func TestResolveArtifactPath_UnknownLayout(t *testing.T) {
	_, err := resolveArtifactPath("/src/foo.md", "foo", "pdf", "/tmp", "banana")
	if err == nil {
		t.Errorf("expected error for unknown layout")
	}
}

func TestChapterMode(t *testing.T) {
	// Neither cfg nor req -> false.
	if chapterMode(Config{}, &Request{}) {
		t.Errorf("chapterMode should default to false")
	}
	// Cfg true, req unset -> true.
	if !chapterMode(Config{AudioByChapter: true}, &Request{}) {
		t.Errorf("cfg default not applied")
	}
	// Req explicit false wins over cfg true.
	f := false
	if chapterMode(Config{AudioByChapter: true}, &Request{AudioByChapter: &f}) {
		t.Errorf("req override not applied")
	}
	// Req explicit true wins over cfg false.
	tr := true
	if !chapterMode(Config{AudioByChapter: false}, &Request{AudioByChapter: &tr}) {
		t.Errorf("req override not applied")
	}
}

func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Hello World", "hello-world"},
		{"Chapter 1: Intro", "chapter-1-intro"},
		{"CamelCaseTitle", "camelcasetitle"},
		{"Multi--Dashes__Under", "multi-dashes-under"},
		{"", "untitled"},
		{"     ", "untitled"},
	}
	for _, c := range cases {
		got := slugify(c.in)
		if got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
