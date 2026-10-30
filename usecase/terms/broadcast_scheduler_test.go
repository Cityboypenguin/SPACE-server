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
	future     []*model.TermsOfService
	err        error
	current    *model.TermsOfService
	currentErr error
}

func (f *fakeTermsRepo) FindFuture(_ context.Context) ([]*model.TermsOfService, error) {
	return f.future, f.err
}

func (f *fakeTermsRepo) FindCurrent(_ context.Context) (*model.TermsOfService, error) {
	return f.current, f.currentErr
}

// claimOnce は BroadcastOnce の偽物。取れた version を記録し、2度目からは false を返す。
type claimOnce struct {
	mu      sync.Mutex
	claimed map[string]bool
	err     error
}

func newClaimOnce() *claimOnce { return &claimOnce{claimed: map[string]bool{}} }

func (c *claimOnce) Claim(_ context.Context, version string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return false, c.err
	}
	if c.claimed[version] {
		return false, nil
	}
	c.claimed[version] = true
	return true, nil
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
	s := NewBroadcastScheduler(&fakeTermsRepo{}, b, nil)

	s.Schedule("v1", time.Now().Add(-time.Hour))

	if got := b.snapshot(); len(got) != 1 || got[0] != "v1" {
		t.Fatalf("events = %v, want a single immediate v1 broadcast", got)
	}
}

func TestSchedule_FiresAtEffectiveDate(t *testing.T) {
	b := &recordingBroadcaster{}
	s := NewBroadcastScheduler(&fakeTermsRepo{}, b, nil)

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
	s := NewBroadcastScheduler(repo, b, nil)

	s.SchedulePending(context.Background())

	got := waitForEvents(t, b, 2)
	if got[0] != "v3" || got[1] != "v4" {
		t.Fatalf("events = %v, want [v3 v4] in effective-date order", got)
	}
}

func TestSchedulePending_IgnoresRepositoryFailure(t *testing.T) {
	b := &recordingBroadcaster{}
	s := NewBroadcastScheduler(&fakeTermsRepo{err: errors.New("db down")}, b, nil)

	// 起動を止めないこと（ログだけ出して戻る）。
	s.SchedulePending(context.Background())

	if got := b.snapshot(); len(got) != 0 {
		t.Fatalf("events = %v, want none when the lookup fails", got)
	}
}

// 台を2つ立てた状況。同じ版のタイマーが両方で発火しても、配るのは1回だけ。
func TestBroadcast_OnlyTheClaimingInstanceSends(t *testing.T) {
	once := newClaimOnce()
	b1, b2 := &recordingBroadcaster{}, &recordingBroadcaster{}
	s1 := NewBroadcastScheduler(&fakeTermsRepo{}, b1, once)
	s2 := NewBroadcastScheduler(&fakeTermsRepo{}, b2, once)

	s1.Schedule("v9", time.Now().Add(-time.Hour))
	s2.Schedule("v9", time.Now().Add(-time.Hour))

	total := len(b1.snapshot()) + len(b2.snapshot())
	if total != 1 {
		t.Fatalf("broadcasts = %d (b1=%v b2=%v), want exactly 1 across both instances",
			total, b1.snapshot(), b2.snapshot())
	}
}

// 門番が応答しないときは配るほうへ倒す。届かないほうが害が大きいため。
func TestBroadcast_FallsBackToSendingWhenTheClaimFails(t *testing.T) {
	once := newClaimOnce()
	once.err = errors.New("redis down")
	b := &recordingBroadcaster{}
	s := NewBroadcastScheduler(&fakeTermsRepo{}, b, once)

	s.Schedule("v10", time.Now().Add(-time.Hour))

	if got := b.snapshot(); len(got) != 1 || got[0] != "v10" {
		t.Fatalf("events = %v, want v10 broadcast despite the claim failure", got)
	}
}

// 発効時刻を停止中に跨いだ版を、起動時に拾う。FindFuture は未来しか返さないので、
// これが無いとその版は誰にも配られない。
func TestSchedulePending_CatchesUpOnAVersionThatBecameEffectiveWhileDown(t *testing.T) {
	once := newClaimOnce()
	repo := &fakeTermsRepo{current: &model.TermsOfService{
		Version:       "v11",
		EffectiveDate: time.Now().Add(-time.Minute),
	}}
	b := &recordingBroadcaster{}
	s := NewBroadcastScheduler(repo, b, once)

	s.SchedulePending(context.Background())

	if got := b.snapshot(); len(got) != 1 || got[0] != "v11" {
		t.Fatalf("events = %v, want a catch-up broadcast of v11", got)
	}
}

// 拾い直しは再起動を跨いでも1回だけ。印が残っているので2度目は配らない。
func TestSchedulePending_CatchUpDoesNotRepeatOnEveryBoot(t *testing.T) {
	once := newClaimOnce()
	repo := &fakeTermsRepo{current: &model.TermsOfService{
		Version:       "v12",
		EffectiveDate: time.Now().Add(-time.Minute),
	}}

	b1 := &recordingBroadcaster{}
	NewBroadcastScheduler(repo, b1, once).SchedulePending(context.Background())
	b2 := &recordingBroadcaster{}
	NewBroadcastScheduler(repo, b2, once).SchedulePending(context.Background())

	if total := len(b1.snapshot()) + len(b2.snapshot()); total != 1 {
		t.Fatalf("broadcasts = %d, want exactly 1 across both boots", total)
	}
}

// 門番が居ない構成（台が1つ）では拾い直しをしない。印が無いので、やると
// 起動のたびに配ってしまう。
func TestSchedulePending_SkipsCatchUpWithoutAClaim(t *testing.T) {
	repo := &fakeTermsRepo{current: &model.TermsOfService{
		Version:       "v13",
		EffectiveDate: time.Now().Add(-time.Minute),
	}}
	b := &recordingBroadcaster{}
	NewBroadcastScheduler(repo, b, nil).SchedulePending(context.Background())

	if got := b.snapshot(); len(got) != 0 {
		t.Fatalf("events = %v, want no catch-up without a claim", got)
	}
}
