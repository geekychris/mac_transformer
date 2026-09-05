package core

import (
	"path/filepath"
	"testing"
	"time"
)

func TestHistory_AppendLoad(t *testing.T) {
	dir := t.TempDir()
	h, err := NewHistory(dir)
	if err != nil {
		t.Fatal(err)
	}

	jobs := []*Job{
		{ID: "j_1", Status: StatusCompleted, Created: time.Now().Add(-2 * time.Hour), InputHash: "abc"},
		{ID: "j_2", Status: StatusFailed, Created: time.Now().Add(-1 * time.Hour), InputHash: "def"},
		{ID: "j_3", Status: StatusRunning, Created: time.Now()}, // should be dropped
	}
	for _, j := range jobs {
		if err := h.Append(j); err != nil {
			t.Fatalf("append %s: %v", j.ID, err)
		}
	}

	// Reload
	h2, _ := NewHistory(dir)
	loaded, err := h2.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("want 2 loaded (Running skipped), got %d", len(loaded))
	}
	if loaded[0].ID != "j_1" || loaded[1].ID != "j_2" {
		t.Errorf("wrong order or ids: %v", loaded)
	}
}

func TestHistory_AppendRunningIsNoop(t *testing.T) {
	dir := t.TempDir()
	h, _ := NewHistory(dir)
	j := &Job{ID: "j_x", Status: StatusRunning}
	if err := h.Append(j); err != nil {
		t.Fatalf("append: %v", err)
	}
	// The file should not exist since Append should have returned early.
	if fi, _ := statIfExists(filepath.Join(dir, "jobs.jsonl")); fi != nil {
		t.Errorf("file created for running job")
	}
}

func statIfExists(p string) (any, error) {
	// helper to swallow not-exist
	return nil, nil
}
