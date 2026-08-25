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
}

type Tracker struct {
	mu     sync.Mutex
	status Status
}

func NewTracker() *Tracker {
	return &Tracker{status: Status{State: StateIdle}}
}

func (t *Tracker) Get() Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// Start rejects the call with apperr.Conflict if a run is already in progress.
// Otherwise it transitions to RUNNING, returns that snapshot immediately, and
// runs run in the background against a fresh context.Background() (the request
// context would be cancelled once the mutation response is sent).
func (t *Tracker) Start(year int, run func(ctx context.Context) (imported, skipped int, err error)) (Status, error) {
	t.mu.Lock()
	if t.status.State == StateRunning {
		t.mu.Unlock()
		return Status{}, apperr.Conflict("既にインポートを実行中です")
	}
	now := time.Now()
	t.status = Status{State: StateRunning, Year: year, StartedAt: &now}
	snapshot := t.status
	t.mu.Unlock()

	go func() {
		bgCtx := context.Background()
		imported, skipped, err := run(bgCtx)
		finished := time.Now()

		t.mu.Lock()
		defer t.mu.Unlock()
		startedAt := t.status.StartedAt
		if err != nil {
			t.status = Status{State: StateFailed, Year: year, ErrorMessage: err.Error(), StartedAt: startedAt, FinishedAt: &finished}
			logger.Log.Error().Err(err).Int("year", year).Msg("course import failed")
			return
		}
		t.status = Status{State: StateSucceeded, Year: year, Imported: imported, Skipped: skipped, StartedAt: startedAt, FinishedAt: &finished}
	}()

	return snapshot, nil
}
