package terms

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type recordingBroadcaster struct {
	mu     sync.Mutex
	events []string // 配信された version を受信順に積む
}

func (b *recordingBroadcaster) Broadcast(eventType string, data map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if eventType != termsUpdatedEvent {
		return
	}
	version, _ := data["version"].(string)
	b.events = append(b.events, version)
}

func (b *recordingBroadcaster) snapshot() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.events...)
}

type fakeTermsRepo struct {
	repository.TermsRepository
	future []*model.TermsOfService
	err    error
}

func (f *fakeTermsRepo) FindFuture(_ context.Context) ([]*model.TermsOfService, error) {
	return f.future, f.err
}

// waitForEvents は非同期な time.AfterFunc の発火を短くポーリングして待つ。
func waitForEvents(t *testing.T, b *recordingBroadcaster, want int) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := b.snapshot()
		if len(got) >= want {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("got %d broadcasts, want %d (events: %v)", len(got), want, got)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSchedule_BroadcastsImmediatelyWhenAlreadyEffective(t *testing.T) {
	b := &recordingBroadcaster{}
	s := NewBroadcastScheduler(&fakeTermsRepo{}, b)

	s.Schedule("v1", time.Now().Add(-time.Hour))

	if got := b.snapshot(); len(got) != 1 || got[0] != "v1" {
		t.Fatalf("events = %v, want a single immediate v1 broadcast", got)
	}
}

func TestSchedule_FiresAtEffectiveDate(t *testing.T) {
	b := &recordingBroadcaster{}
	s := NewBroadcastScheduler(&fakeTermsRepo{}, b)

	s.Schedule("v2", time.Now().Add(20*time.Millisecond))

	if got := b.snapshot(); len(got) != 0 {
		t.Fatalf("events = %v, want nothing before the effective date", got)
	}
	if got := waitForEvents(t, b, 1); got[0] != "v2" {
		t.Fatalf("events = %v, want v2", got)
	}
}

// 起動時と作成時が同じ1実装を通ることの確認。SchedulePending は「未来日付で
// 登録済みの各版」に Schedule を張るだけで、別経路の配信ロジックを持たない。
func TestSchedulePending_SchedulesEveryFutureVersion(t *testing.T) {
	b := &recordingBroadcaster{}
	repo := &fakeTermsRepo{future: []*model.TermsOfService{
		{Version: "v3", EffectiveDate: time.Now().Add(10 * time.Millisecond)},
		{Version: "v4", EffectiveDate: time.Now().Add(20 * time.Millisecond)},
	}}
	s := NewBroadcastScheduler(repo, b)

	s.SchedulePending(context.Background())

	got := waitForEvents(t, b, 2)
	if got[0] != "v3" || got[1] != "v4" {
		t.Fatalf("events = %v, want [v3 v4] in effective-date order", got)
	}
}

func TestSchedulePending_IgnoresRepositoryFailure(t *testing.T) {
	b := &recordingBroadcaster{}
	s := NewBroadcastScheduler(&fakeTermsRepo{err: errors.New("db down")}, b)

	// 起動を止めないこと（ログだけ出して戻る）。
	s.SchedulePending(context.Background())

	if got := b.snapshot(); len(got) != 0 {
		t.Fatalf("events = %v, want none when the lookup fails", got)
	}
}
