package core

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// History is an append-only, JSON-lines persistent store of finished jobs.
// The current-session Queue is source of truth for in-flight state; the
// History lets it survive restarts so retroactive publish / duplicate
// detection / stats can span sessions.
type History struct {
	path     string
	mu       sync.Mutex
	maxLines int
}

func NewHistory(dir string) (*History, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &History{
		path:     filepath.Join(dir, "jobs.jsonl"),
		maxLines: 5000, // ~5 MB at typical job sizes; plenty for personal use
	}, nil
}

// Append writes a completed/failed/cancelled job to the log. Skipped for
// still-running jobs to avoid partial state on disk.
func (h *History) Append(job *Job) error {
	if job.Status != StatusCompleted && job.Status != StatusFailed && job.Status != StatusCancelled {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	f, err := os.OpenFile(h.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(job)
}

// Load reads every persisted job, sorted by creation time. Called once at
// startup to hydrate the Queue. Returns an empty slice if the file doesn't
// exist yet.
func (h *History) Load() ([]*Job, error) {
	f, err := os.Open(h.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var jobs []*Job
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 8*1024*1024) // handle big jobs
	for sc.Scan() {
		var j Job
		if err := json.Unmarshal(sc.Bytes(), &j); err != nil {
			continue // skip corrupt lines; don't lose the whole log
		}
		jobs = append(jobs, &j)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Created.Before(jobs[j].Created) })
	if len(jobs) > h.maxLines {
		jobs = jobs[len(jobs)-h.maxLines:]
	}
	return jobs, sc.Err()
}

