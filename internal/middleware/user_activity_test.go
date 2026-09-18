package middleware

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/async"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// fakeUserRepo は活動記録の2メソッドだけを数える。埋め込んだ interface は nil の
// ままだが、記録がそれ以外を呼ばないことも含めてテストになる（呼べば panic する）。
type fakeUserRepo struct {
	repository.UserRepository

	mu         sync.Mutex
	lastActive []int64
	dates      []string
	err        error
}

func (f *fakeUserRepo) UpdateLastActiveAt(_ context.Context, _ int64, now int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastActive = append(f.lastActive, now)
	return f.err
}

func (f *fakeUserRepo) LogActivityDate(_ context.Context, _ int64, jstDate string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dates = append(f.dates, jstDate)
	return f.err
}

func (f *fakeUserRepo) writes() (int, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.lastActive), append([]string(nil), f.dates...)
}

// テスト用のレコーダ。時計を手で進められるようにし、書き込みは同期で走らせる
// （async=nil。非同期そのものは internal/async のテストが見ている）。
func newTestRecorder(repo repository.UserRepository, now *time.Time) *UserActivityRecorder {
	r := NewUserActivityRecorder(repo, nil)
	r.now = func() time.Time { return *now }
	return r
}

// 同じユーザーが短時間に何度アクセスしても、DB へは1回しか書かないこと。
func TestUserActivityRecorder_SkipsRepeatedRequests(t *testing.T) {
	repo := &fakeUserRepo{}
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	r := newTestRecorder(repo, &now)

	if !r.Record(context.Background(), 1) {
		t.Fatal("the first request should record the activity")
	}
	for i := 0; i < 50; i++ {
		now = now.Add(time.Second)
		if r.Record(context.Background(), 1) {
			t.Fatalf("request %d should have been skipped", i)
		}
	}

	if count, _ := repo.writes(); count != 1 {
		t.Fatalf("expected exactly 1 write for 51 requests, got %d", count)
	}
}

// 間引く間隔を過ぎたら、また書きに行くこと。
func TestUserActivityRecorder_WritesAgainAfterTheInterval(t *testing.T) {
	repo := &fakeUserRepo{}
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	r := newTestRecorder(repo, &now)

	r.Record(context.Background(), 1)
	now = now.Add(userActivityInterval)
	if !r.Record(context.Background(), 1) {
		t.Fatal("the request after the interval should record again")
	}

	if count, _ := repo.writes(); count != 2 {
		t.Fatalf("expected 2 writes, got %d", count)
	}
}

// 間引く間隔の中でも、JST の日付が変わったら書くこと
// （活動日の行が1日ぶん丸ごと落ちるのを防ぐ）。
func TestUserActivityRecorder_WritesOnJSTDateChange(t *testing.T) {
	repo := &fakeUserRepo{}
	// 14:58 UTC = 23:58 JST。2分後に JST の日付が変わる。
	now := time.Date(2026, 9, 18, 14, 58, 0, 0, time.UTC)
	r := newTestRecorder(repo, &now)

	r.Record(context.Background(), 1)
	now = now.Add(3 * time.Minute) // まだ間引く間隔の中
	if !r.Record(context.Background(), 1) {
		t.Fatal("crossing the JST date boundary must record even within the interval")
	}

	count, dates := repo.writes()
	if count != 2 {
		t.Fatalf("expected 2 writes, got %d", count)
	}
	if dates[0] != "2026-09-18" || dates[1] != "2026-09-19" {
		t.Fatalf("expected consecutive JST dates, got %v", dates)
	}
}

// 覚えている印が掃除され、ユーザー数に比例して増え続けないこと。
func TestUserActivityRecorder_SweepsStaleMarks(t *testing.T) {
	repo := &fakeUserRepo{}
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	r := newTestRecorder(repo, &now)

	for id := int64(1); id <= 100; id++ {
		r.Record(context.Background(), id)
	}
	r.mu.Lock()
	before := len(r.last)
	r.mu.Unlock()
	if before != 100 {
		t.Fatalf("expected 100 marks, got %d", before)
	}

	// 掃除の間隔を過ぎてから誰か1人が来れば、古い印はまとめて捨てられる。
	now = now.Add(userActivitySweepInterval + time.Minute)
	r.Record(context.Background(), 1)

	r.mu.Lock()
	after := len(r.last)
	r.mu.Unlock()
	if after != 1 {
		t.Fatalf("expected the sweep to leave only the active user, got %d marks", after)
	}
}

// 書き込みが失敗しても呼び出し側へは伝播せず、次の間隔まで撃ち直さないこと
// （失敗はログに残す。以前は戻り値を捨てていて気づけなかった）。
func TestUserActivityRecorder_SurvivesWriteFailures(t *testing.T) {
	repo := &fakeUserRepo{err: errors.New("db down")}
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	r := newTestRecorder(repo, &now)

	r.Record(context.Background(), 1)
	now = now.Add(time.Minute)
	if r.Record(context.Background(), 1) {
		t.Fatal("a failed write must not make the next request retry immediately")
	}
	if count, _ := repo.writes(); count != 1 {
		t.Fatalf("expected 1 attempt, got %d", count)
	}
}

// Runner 経由でも記録され、停止時の Wait で取りこぼさないこと。
func TestUserActivityRecorder_RunsThroughTheAsyncRunner(t *testing.T) {
	repo := &fakeUserRepo{}
	runner := async.NewRunner("test")
	r := NewUserActivityRecorder(repo, runner)

	r.Record(context.Background(), 1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runner.Wait(ctx); err != nil {
		t.Fatalf("pending activity writes did not drain: %v", err)
	}
	if count, _ := repo.writes(); count != 1 {
		t.Fatalf("expected the activity to be written once, got %d", count)
	}
}
