package courseimport

import (
	"context"
	"testing"
	"time"
)

func TestShutdownCancelsAndWaitsForImport(t *testing.T) {
	tracker := NewTracker(nil)
	started := make(chan struct{})
	finished := make(chan struct{})
	_, err := tracker.Start(2026, func(ctx context.Context, report func(int, int)) (int, int, error) {
		close(started)
		for i := 0; i < 100; i++ {
			report(i, 100)
		}
		<-ctx.Done()
		close(finished)
		return 0, 0, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := tracker.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("shutdown returned before the import exited")
	}
	if state := tracker.Get().State; state != StateFailed {
		t.Fatalf("state after cancellation = %s", state)
	}
	if _, err := tracker.Start(2027, func(context.Context, func(int, int)) (int, int, error) {
		return 0, 0, nil
	}); err == nil {
		t.Fatal("a stopped tracker accepted a new import")
	}
}
