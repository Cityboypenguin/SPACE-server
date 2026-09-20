package courseimport

import (
	"context"
	"testing"
	"time"
)

func TestShutdownCancelsAndWaitsForImport(t *testing.T) {
	tracker := NewTracker(nil, nil)
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

// --- 台をまたいだ排他（項目9） --------------------------------------------

// TestStart_IsRejectedWhenAnotherInstanceIsRunning は、別の台が取り込み中なら
// 始められないことを確かめる。
//
// 排他が自分の台のメモリしか見ていないと、管理者2人が別々の台に当たったときに
// 同じ年度の取り込みが2本走る。1台で動かしている間は起きないので、共有の記録を
// 挟んだ2つの Tracker で確かめる。
func TestStart_IsRejectedWhenAnotherInstanceIsRunning(t *testing.T) {
	// 2台が同じ記録を見ている状態を作る。
	shared := NewMemoryStore()
	instanceA := NewTracker(shared, nil)
	instanceB := NewTracker(shared, nil)

	release := make(chan struct{})
	defer close(release)

	if _, err := instanceA.Start(2026, func(context.Context, func(int, int)) (int, int, error) {
		<-release
		return 1, 0, nil
	}); err != nil {
		t.Fatalf("1台目が始められない: %v", err)
	}

	if _, err := instanceB.Start(2026, func(context.Context, func(int, int)) (int, int, error) {
		t.Error("2台目の取り込みが走ってしまった")
		return 0, 0, nil
	}); err == nil {
		t.Fatal("別の台が実行中なのに2本目を受け付けた")
	}
}

// TestGet_SeesAnotherInstancesProgress は、取り込みを始めた台とは別の台に
// 聞いても状態が見えることを確かめる。
//
// 状態が自分の台のメモリにしか無いと、管理画面が別の台へ繋がったときに
// IDLE のままに見える（動いているのに「まだ始まっていない」と出る）。
func TestGet_SeesAnotherInstancesProgress(t *testing.T) {
	shared := NewMemoryStore()
	instanceA := NewTracker(shared, nil)
	instanceB := NewTracker(shared, nil)

	reported := make(chan struct{})
	release := make(chan struct{})
	defer close(release)

	if _, err := instanceA.Start(2026, func(_ context.Context, report func(int, int)) (int, int, error) {
		report(30, 100)
		close(reported)
		<-release
		return 0, 0, nil
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	<-reported

	// 取り込みを走らせていない台に聞く。
	got := instanceB.Get()
	if got.State != StateRunning {
		t.Fatalf("state = %s, want %s（別の台の取り込みが見えていない）", got.State, StateRunning)
	}
	if got.Year != 2026 {
		t.Fatalf("year = %d, want 2026", got.Year)
	}
	if got.Processed != 30 || got.Total != 100 {
		t.Fatalf("progress = %d/%d, want 30/100（進捗が共有されていない）", got.Processed, got.Total)
	}
}

// TestStart_IsAllowedOnceTheOtherInstanceFinishes は、排他が終了後に
// ちゃんと解けることを確かめる。解けないと、以降どの台からも取り込めなくなる。
func TestGet_SeesAnotherInstancesResultAndAllowsTheNextRun(t *testing.T) {
	shared := NewMemoryStore()
	instanceA := NewTracker(shared, nil)
	instanceB := NewTracker(shared, nil)

	done := make(chan struct{})
	if _, err := instanceA.Start(2026, func(context.Context, func(int, int)) (int, int, error) {
		return 7, 2, nil
	}); err != nil {
		t.Fatalf("start: %v", err)
	}

	// 終了が記録されるまで待つ（Start は走らせて即戻る）。
	go func() {
		for {
			if instanceB.Get().State == StateSucceeded {
				close(done)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("別の台から結果が見えない（state = %s）", instanceB.Get().State)
	}

	got := instanceB.Get()
	if got.Imported != 7 || got.Skipped != 2 {
		t.Fatalf("result = imported %d / skipped %d, want 7 / 2", got.Imported, got.Skipped)
	}

	// 終わっているので次を始められる。
	if _, err := instanceB.Start(2027, func(context.Context, func(int, int)) (int, int, error) {
		return 0, 0, nil
	}); err != nil {
		t.Fatalf("終了後に次の取り込みを始められない（排他が解けていない）: %v", err)
	}
}
