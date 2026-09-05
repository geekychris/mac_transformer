package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// stubDispatcher is a minimal Dispatcher-shaped fake for queue tests. Just
// marks the job completed after a small delay so we can observe transitions
// without pulling in the real plugin registry.
type stubDispatcher struct{ delay time.Duration }

func (d *stubDispatcher) Run(ctx context.Context, job *Job) {
	select {
	case <-time.After(d.delay):
		job.Status = StatusCompleted
	case <-ctx.Done():
		job.Status = StatusCancelled
	}
}

// The Dispatcher.Run signature matches core.Dispatcher.Run; we just wrap
// stubDispatcher into the real type for the queue.
func newTestDispatcher(delay time.Duration) *Dispatcher {
	// Queue calls d.Run(ctx, job) inside its own loop; the real Dispatcher's
	// Run walks req.Outputs, so we need a way to inject behavior. Simplest:
	// use nil Registry and let Run bail out early. Better: bypass by NOT
	// going through core.Dispatcher.Run at all. But NewQueue requires a
	// *Dispatcher, so we shim.
	return &Dispatcher{Registry: nil, Config: nil}
}

func TestQueue_CancelPending(t *testing.T) {
	// Build a queue with a real dispatcher backed by a nil registry. Jobs
	// will immediately fail on Submit(). We just verify Cancel returns
	// true for pending jobs.
	q := NewQueue(newTestDispatcher(10*time.Second), nil)
	// Enqueue a fake job directly (bypass Submit's channel to keep it pending).
	j := &Job{ID: "j_pending", Status: StatusPending, Created: time.Now()}
	q.mu.Lock()
	q.jobs[j.ID] = j
	q.fifo = append(q.fifo, j.ID)
	q.mu.Unlock()

	if !q.Cancel(j.ID) {
		t.Errorf("Cancel(pending) should return true")
	}
	if j.Status != StatusCancelled {
		t.Errorf("Status after Cancel = %s, want cancelled", j.Status)
	}
}

func TestQueue_CancelUnknownJob(t *testing.T) {
	q := NewQueue(newTestDispatcher(0), nil)
	if q.Cancel("j_nonexistent") {
		t.Errorf("Cancel(unknown) should return false")
	}
}

func TestQueue_FindByInputHash(t *testing.T) {
	q := NewQueue(newTestDispatcher(0), nil)
	j1 := &Job{ID: "j_1", Status: StatusCompleted, InputHash: "abc", Created: time.Now().Add(-time.Hour)}
	j2 := &Job{ID: "j_2", Status: StatusCompleted, InputHash: "def", Created: time.Now()}
	q.mu.Lock()
	q.jobs[j1.ID] = j1
	q.jobs[j2.ID] = j2
	q.fifo = []string{j1.ID, j2.ID}
	q.mu.Unlock()

	if got := q.FindByInputHash("abc"); got == nil || got.ID != "j_1" {
		t.Errorf("expected j_1, got %v", got)
	}
	if got := q.FindByInputHash("zzz"); got != nil {
		t.Errorf("expected nil for miss, got %v", got)
	}
	if got := q.FindByInputHash(""); got != nil {
		t.Errorf("expected nil for empty hash")
	}
}

func TestHashInput(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	_ = os.WriteFile(a, []byte("hello"), 0o644)
	_ = os.WriteFile(b, []byte("hello"), 0o644)

	if hashInput(a) != hashInput(b) {
		t.Errorf("same content should produce same hash")
	}
	if hashInput(a) == "" {
		t.Errorf("hash should be non-empty for existing file")
	}
	if hashInput(filepath.Join(dir, "nope.txt")) != "" {
		t.Errorf("hash of missing file should be empty")
	}
}
