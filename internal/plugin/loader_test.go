package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifest(t *testing.T) {
	dir := t.TempDir()
	yaml := `
name: test-plugin
description: test
version: 1
capabilities:
  - from: [docx]
    to:   [pdf]
    priority: 50
requires: []
run:
  kind: command
  argv: [echo, hi]
`
	p := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, errs := Load(context.Background(), []string{dir})
	if len(errs) > 0 {
		t.Fatalf("load errors: %v", errs)
	}
	all := reg.All()
	if len(all) != 1 {
		t.Fatalf("want 1 plugin, got %d", len(all))
	}
	if all[0].Manifest.Name != "test-plugin" {
		t.Errorf("wrong name: %s", all[0].Manifest.Name)
	}
	if !all[0].Available {
		t.Errorf("expected available: %s", all[0].Reason)
	}
	matches := reg.Match("docx", "pdf")
	if len(matches) != 1 {
		t.Errorf("Match want 1, got %d", len(matches))
	}
}

func TestExpand(t *testing.T) {
	ctx := &TemplateContext{Input: "/a/b.txt", InputStem: "b", OutputExt: "pdf"}
	cases := []struct {
		in, want string
	}{
		{"{{input}}", "/a/b.txt"},
		{"{{input_stem}}.{{output_ext}}", "b.pdf"},
		{"{{missing | default:fallback}}", "fallback"},
		{"prefix-{{output_ext}}-suffix", "prefix-pdf-suffix"},
	}
	for _, c := range cases {
		got := Expand(c.in, ctx)
		if got != c.want {
			t.Errorf("Expand(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
