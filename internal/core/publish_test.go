package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePublish_ConfigDefaults(t *testing.T) {
	cfg := Config{
		PublishToBooks:         true,
		DefaultBooksCollection: "Default",
		PublishToMusic:         false,
	}
	req := Request{}
	po := resolvePublish(cfg, &req)
	if !po.Books {
		t.Errorf("Books default not applied")
	}
	if po.BooksCollection != "Default" {
		t.Errorf("collection default not applied: %s", po.BooksCollection)
	}
	if po.Music {
		t.Errorf("Music should be false")
	}
}

func TestResolvePublish_RequestOverrides(t *testing.T) {
	cfg := Config{PublishToBooks: false}
	req := Request{
		PublishToBooks:  boolPtr(true),
		BooksCollection: "OneOff",
	}
	po := resolvePublish(cfg, &req)
	if !po.Books {
		t.Errorf("request override not applied")
	}
	if po.BooksCollection != "OneOff" {
		t.Errorf("collection override not applied")
	}
}

func TestResolvePublish_RequestExplicitFalse(t *testing.T) {
	cfg := Config{PublishToBooks: true}
	f := false
	req := Request{PublishToBooks: &f}
	po := resolvePublish(cfg, &req)
	if po.Books {
		t.Errorf("request should have disabled Books, got true")
	}
}

func TestResolvePublish_TagsReplace(t *testing.T) {
	cfg := Config{DefaultTags: []string{"Global"}}
	req := Request{Tags: []string{"PerJob"}}
	po := resolvePublish(cfg, &req)
	if len(po.Tags) != 1 || po.Tags[0] != "PerJob" {
		t.Errorf("tags should be per-job, got %v", po.Tags)
	}
}

func TestSetFinderTags(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(f, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setFinderTags(f, []string{"Alpha", "Beta"}); err != nil {
		t.Fatalf("setFinderTags: %v", err)
	}
	// Read back xattr via os.Getxattr equivalent (not in stdlib on darwin).
	// Cheapest sanity: run `xattr -p` and check that our two names appear.
	// We don't verify plist parsing here — just presence.
	// The test setter writes an XML plist that must contain the names.
	// Read the file directly using unix syscall isn't portable; use exec.
	out, err := runCmd("xattr", "-p", "com.apple.metadata:_kMDItemUserTags", f)
	if err != nil {
		t.Fatalf("xattr read: %v", err)
	}
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") {
		t.Errorf("tags not persisted: %s", out)
	}
}

func TestApplyPostProcessing_ReplacesTagsOnRerun(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(f, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	art := Artifact{Path: f, Format: FormatTXT}
	// First run: tag with Alpha.
	po1 := PublishOptions{Tags: []string{"Alpha"}}
	ApplyPostProcessing(t.Context(), &art, po1, "")
	// Second run: tag with Beta. Expect only one tags entry.
	po2 := PublishOptions{Tags: []string{"Beta"}}
	ApplyPostProcessing(t.Context(), &art, po2, "")
	tagResults := 0
	for _, p := range art.Publish {
		if p.Target == "tags" {
			tagResults++
		}
	}
	if tagResults != 1 {
		t.Errorf("expected 1 tags result after re-run, got %d: %+v", tagResults, art.Publish)
	}
	if art.Tags == nil || art.Tags[0] != "Beta" {
		t.Errorf("Tags should be the latest, got %v", art.Tags)
	}
}
