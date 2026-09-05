package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"sync"
	"time"
)

// Queue is an in-memory job queue. Jobs are executed one at a time in
// submission order (avoids two soffice or Word instances racing over the
// same document library).
type Queue struct {
	dispatcher *Dispatcher
	history    *History

	mu      sync.RWMutex
	jobs    map[string]*Job
	fifo    []string
	cancels map[string]context.CancelFunc // set while a job is running

	ch chan *Job

	subMu sync.Mutex
	subs  []chan Event
}

// Event is a queue-wide notification: created / started / progress / done /
// failed. Consumers get it via Subscribe(); each subscriber gets an
// independent channel.
type Event struct {
	Type string `json:"type"`
	Job  *Job   `json:"job"`
}

func NewQueue(d *Dispatcher, history *History) *Queue {
	q := &Queue{
		dispatcher: d,
		history:    history,
		jobs:       map[string]*Job{},
		cancels:    map[string]context.CancelFunc{},
		ch:         make(chan *Job, 64),
	}
	// Hydrate prior jobs so retroactive publish / history browse / dedup
	// all work across restarts.
	if history != nil {
		if prior, err := history.Load(); err == nil {
			for _, j := range prior {
				q.jobs[j.ID] = j
				q.fifo = append(q.fifo, j.ID)
			}
		}
	}
	// The dispatcher pings us on every state transition; forward those as
	// "progress" events so subscribers can re-render mid-flight.
	if d.Progress == nil {
		d.Progress = func(job *Job) {
			q.emit(Event{Type: "progress", Job: job})
		}
	}
	go q.loop()
	return q
}

func (q *Queue) Submit(req Request) *Job {
	id := newID()
	job := &Job{
		ID:        id,
		Request:   req,
		Status:    StatusPending,
		Created:   timeNow(),
		InputHash: hashInput(req.InputPath),
	}
	q.mu.Lock()
	q.jobs[id] = job
	q.fifo = append(q.fifo, id)
	q.mu.Unlock()
	q.emit(Event{Type: "created", Job: job})
	q.ch <- job
	return job
}

// FindByInputHash walks jobs (newest first) and returns the most recent
// job with the given input content hash. Used by callers before submitting
// to warn about duplicates.
func (q *Queue) FindByInputHash(hash string) *Job {
	if hash == "" {
		return nil
	}
	q.mu.RLock()
	defer q.mu.RUnlock()
	for i := len(q.fifo) - 1; i >= 0; i-- {
		j := q.jobs[q.fifo[i]]
		if j != nil && j.InputHash == hash {
			return j
		}
	}
	return nil
}

func (q *Queue) Get(id string) (*Job, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	j, ok := q.jobs[id]
	return j, ok
}

// EmitProgress pushes a "progress" event carrying the given job. Callers
// that mutate a job outside the dispatcher (e.g. a retroactive publish
// action) use this to notify subscribers so the UI re-renders.
func (q *Queue) EmitProgress(job *Job) {
	q.emit(Event{Type: "progress", Job: job})
}

func (q *Queue) List() []*Job {
	q.mu.RLock()
	defer q.mu.RUnlock()
	out := make([]*Job, 0, len(q.fifo))
	for _, id := range q.fifo {
		out = append(out, q.jobs[id])
	}
	return out
}

// Subscribe returns a fresh channel of Events. Close the channel via the
// returned cancel func when done.
func (q *Queue) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 16)
	q.subMu.Lock()
	q.subs = append(q.subs, ch)
	q.subMu.Unlock()
	cancel := func() {
		q.subMu.Lock()
		for i, s := range q.subs {
			if s == ch {
				q.subs = append(q.subs[:i], q.subs[i+1:]...)
				break
			}
		}
		q.subMu.Unlock()
		close(ch)
	}
	return ch, cancel
}

func (q *Queue) emit(ev Event) {
	q.subMu.Lock()
	subs := make([]chan Event, len(q.subs))
	copy(subs, q.subs)
	q.subMu.Unlock()
	for _, s := range subs {
		select {
		case s <- ev:
		default:
			// Drop for slow subscribers rather than block the queue.
		}
	}
}

func (q *Queue) loop() {
	for job := range q.ch {
		// Check for pre-cancellation (Cancel was called before we picked
		// the job off the channel).
		q.mu.Lock()
		if _, alreadyCancelled := q.cancels[job.ID]; !alreadyCancelled && job.Status == StatusCancelled {
			q.mu.Unlock()
			q.emit(Event{Type: "cancelled", Job: job})
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		q.cancels[job.ID] = cancel
		q.mu.Unlock()

		q.emit(Event{Type: "started", Job: job})
		q.dispatcher.Run(ctx, job)

		q.mu.Lock()
		delete(q.cancels, job.ID)
		q.mu.Unlock()
		cancel()

		typ := "completed"
		switch job.Status {
		case StatusFailed:
			typ = "failed"
		case StatusCancelled:
			typ = "cancelled"
		}
		// Persist the final state before notifying subscribers — that way
		// a subscriber-triggered reload still finds it on disk.
		if q.history != nil {
			_ = q.history.Append(job)
		}
		q.emit(Event{Type: typ, Job: job})
	}
}

// Cancel signals a running job to stop, or marks a pending job as cancelled
// so the loop skips it. No-op if the job is already done. Returns true if
// something was cancelled.
func (q *Queue) Cancel(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	job, ok := q.jobs[id]
	if !ok {
		return false
	}
	switch job.Status {
	case StatusCompleted, StatusFailed, StatusCancelled:
		return false
	}
	if cancel, running := q.cancels[id]; running {
		cancel()
		job.Message = "cancelled by user"
		return true
	}
	// Pending — just mark it. loop() will skip it when it dequeues.
	job.Status = StatusCancelled
	job.Message = "cancelled before start"
	return true
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "j_" + hex.EncodeToString(b[:])
}

// HashInputForRequest is the exported form so httpapi can hash before
// submitting.
func HashInputForRequest(path string) string { return hashInput(path) }

// hashInput returns a short sha256 prefix of the file at path, for
// duplicate detection. Empty on error — dedup silently skips unknown files
// rather than fail the whole submit path.
func hashInput(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}

func timeNow() time.Time { return time.Now() }
