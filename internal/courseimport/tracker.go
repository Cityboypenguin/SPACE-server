// Package courseimport tracks the status of the (long-running, sequential,
// curl-shelling) course scrape-and-import job so it can be triggered from a
// GraphQL mutation without blocking the request/response cycle. Status is kept
// in process memory only (lost on server restart) — this is an operator tool
// run occasionally by admins, not something that needs durable job history.
package courseimport

import (
	"context"
	"sync"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
)

type State string

const (
	StateIdle      State = "IDLE"
	StateRunning   State = "RUNNING"
	StateSucceeded State = "SUCCEEDED"
	StateFailed    State = "FAILED"
)

type Status struct {
	State        State
	Year         int
	Imported     int
	Skipped      int
	ErrorMessage string
	StartedAt    *time.Time
	FinishedAt   *time.Time
	// Processed and Total describe progress of a RUNNING scrape (rows fetched so
	// far / total rows reported by the source site). Both are 0 until the first
	// progress report arrives, and are left at their last value once the run
	// finishes (State moves to SUCCEEDED/FAILED before the caller can react to it).
	Processed int
	Total     int
}

type Tracker struct {
	mu       sync.Mutex
	status   Status
	onChange func(Status)
}

// NewTracker builds a Tracker that calls onChange (if non-nil) with a snapshot of
// the status every time it changes - Start, SetProgress, and completion - so a
// caller can push updates (e.g. over a GraphQL subscription) instead of relying on
// callers to poll Get.
func NewTracker(onChange func(Status)) *Tracker {
	return &Tracker{status: Status{State: StateIdle}, onChange: onChange}
}

func (t *Tracker) Get() Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// notify reads the current status under the lock and then calls onChange with that
// snapshot after releasing it, so onChange (which may be slow, e.g. fanning out to
// subscribers) never runs while holding the mutex.
func (t *Tracker) notify() {
	t.mu.Lock()
	snapshot := t.status
	t.mu.Unlock()
	if t.onChange != nil {
		t.onChange(snapshot)
	}
}

// SetProgress records how many of the total rows a RUNNING scrape has fetched so
// far. It is a no-op once the run has left the RUNNING state (e.g. a stray report
// arriving after cancellation), so it never resurrects a finished status.
func (t *Tracker) SetProgress(processed, total int) {
	t.mu.Lock()
	if t.status.State != StateRunning {
		t.mu.Unlock()
		return
	}
	t.status.Processed = processed
	t.status.Total = total
	t.mu.Unlock()
	t.notify()
}

// Start rejects the call with apperr.Conflict if a run is already in progress.
// Otherwise it transitions to RUNNING, returns that snapshot immediately, and
// runs run in the background against a fresh context.Background() (the request
// context would be cancelled once the mutation response is sent). run is handed
// t.SetProgress so it can report incremental progress while it works.
func (t *Tracker) Start(year int, run func(ctx context.Context, reportProgress func(processed, total int)) (imported, skipped int, err error)) (Status, error) {
	t.mu.Lock()
	if t.status.State == StateRunning {
		t.mu.Unlock()
		return Status{}, apperr.Conflict("既にインポートを実行中です")
	}
	now := time.Now()
	t.status = Status{State: StateRunning, Year: year, StartedAt: &now}
	snapshot := t.status
	t.mu.Unlock()
	t.notify()

	go func() {
		bgCtx := context.Background()
		imported, skipped, err := run(bgCtx, t.SetProgress)
		finished := time.Now()

		t.mu.Lock()
		startedAt := t.status.StartedAt
		if err != nil {
			t.status = Status{State: StateFailed, Year: year, ErrorMessage: err.Error(), StartedAt: startedAt, FinishedAt: &finished}
			t.mu.Unlock()
			logger.Log.Error().Err(err).Int("year", year).Msg("course import failed")
			t.notify()
			return
		}
		t.status = Status{State: StateSucceeded, Year: year, Imported: imported, Skipped: skipped, StartedAt: startedAt, FinishedAt: &finished}
		t.mu.Unlock()
		t.notify()
	}()

	return snapshot, nil
}
