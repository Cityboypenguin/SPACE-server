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
	hours      []string
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

func (f *fakeUserRepo) LogActivityHour(_ context.Context, _ int64, jstHour string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hours = append(f.hours, jstHour)
	return f.err
}

func (f *fakeUserRepo) writes() (int, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.lastActive), append([]string(nil), f.dates...)
}

func (f *fakeUserRepo) writtenHours() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.hours...)
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

// 間引く間隔の中でも、JST の時間帯が変わったら書くこと
// （その時間帯の行が丸ごと落ちて、時間別グラフのスロットが欠けるのを防ぐ）。
//
// これが無いと 10:58 に書いた人の 11:02 のアクセスが「5分以内」で間引かれ、
// user_activity_hours に11時台の行が残らない。last_active_at ベースの集計が
// 抱えていた「過去の時間帯ほど人数が少なく出る」状態がそのまま再発する。
func TestUserActivityRecorder_WritesOnJSTHourChange(t *testing.T) {
	repo := &fakeUserRepo{}
	// 01:58 UTC = 10:58 JST。4分後に JST の時間帯が変わる（日付は変わらない）。
	now := time.Date(2026, 9, 18, 1, 58, 0, 0, time.UTC)
	r := newTestRecorder(repo, &now)

	if !r.Record(context.Background(), 1) {
		t.Fatal("the first request should record the activity")
	}
	now = now.Add(4 * time.Minute) // まだ間引く間隔（5分）の中
	if !r.Record(context.Background(), 1) {
		t.Fatal("crossing the JST hour boundary must record even within the interval")
	}

	count, dates := repo.writes()
	if count != 2 {
		t.Fatalf("expected 2 writes, got %d", count)
	}
	// 日付は変わっていないので、活動日の行は同じ日付のまま2回（INSERT IGNORE が捨てる）。
	if dates[0] != "2026-09-18" || dates[1] != "2026-09-18" {
		t.Fatalf("activity dates = %v, want the same JST date twice", dates)
	}
	hours := repo.writtenHours()
	if len(hours) != 2 || hours[0] != "2026-09-18 10:00:00" || hours[1] != "2026-09-18 11:00:00" {
		t.Fatalf("activity hours = %v, want 10時台と11時台の2行", hours)
	}
}

// 同じ時間帯の中では、5分の間引きが効いたままであること
// （時間帯で見るようにしても、書き込み回数が毎リクエストに戻ってはいけない）。
func TestUserActivityRecorder_StillThrottlesWithinTheSameHour(t *testing.T) {
	repo := &fakeUserRepo{}
	// 01:00 UTC = 10:00 JST。以降59分ぶん、1分ごとにアクセスする。
	now := time.Date(2026, 9, 18, 1, 0, 0, 0, time.UTC)
	r := newTestRecorder(repo, &now)

	writes := 0
	for i := 0; i < 60; i++ {
		if r.Record(context.Background(), 1) {
			writes++
		}
		now = now.Add(time.Minute)
	}

	// 10:00 の1回目と、5分ごとの11回（10:05..10:55）で12回。
	// 毎分書いていたら 60 回、時間帯だけで判定していたら 1 回になる。
	if writes != 12 {
		t.Fatalf("writes in one hour = %d, want 12 (5分ごと)", writes)
	}
	for _, h := range repo.writtenHours() {
		if h != "2026-09-18 10:00:00" {
			t.Fatalf("activity hour = %q, want すべて 10時台", h)
		}
	}
}

// 間引く間隔の中でも、JST の日付が変わったら書くこと
// （活動日の行が1日ぶん丸ごと落ちるのを防ぐ）。時間帯で見るようになっても
// この規則は保たれる（日付が変われば時間帯も必ず変わる）。
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
	hours := repo.writtenHours()
	if len(hours) != 2 || hours[0] != "2026-09-18 23:00:00" || hours[1] != "2026-09-19 00:00:00" {
		t.Fatalf("activity hours = %v, want 日付をまたいだ2つの時間帯", hours)
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
