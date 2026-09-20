package async

import (
	"context"
	"sync"
	"testing"
	"time"
)

// 投げっぱなしの実行口そのもののテスト。
//
// 何をどこへ配信するかは graph/chat_events_test.go が受け持つ。ここで見るのは
// ctx の扱い・panic の握り・Wait の3点だけ。
//
// # 順序は保証しない（意図的）
//
// 以前はルーム単位のFIFOキューで順序を保っていたが、そこを通る配信から順序の要る
// もの（PubSub の message:added など）を外し、アダプタ側で同期に実行するようにした
// ので、この Runner に順序の保証は要らなくなった。渡した関数は goroutine で即座に
// 走り、どちらが先に終わるかは決まらない。したがって順序のテストも無い。
// 順序が要るものをここへ渡さないこと（チャット配信での判断は usecase/chat/async_events.go のコメント）。

type ctxKey struct{}

func waitForDelivery(t *testing.T, r *Runner) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.Wait(ctx); err != nil {
		t.Fatalf("pending chat events did not drain: %v", err)
	}
}

// リクエストの ctx がキャンセルされても配信は続けられること、かつ ctx に載せた値は
// 引き継がれること。Background() に差し替えると値まで落ちて、通知の保存や監査ログが
// 「誰の操作か」を見失う。
func TestRunner_KeepsContextValuesButNotCancellation(t *testing.T) {
	runner := NewRunner("test")

	var (
		mu      sync.Mutex
		ctxLive bool
		tag     any
	)

	// リクエスト処理中の ctx を模す: 値が載っていて、応答後にキャンセルされる。
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "claims"))
	started := make(chan struct{})
	runner.Go(ctx, "message_sent", func(ctx context.Context) {
		<-started // 配信より先にリクエストが終わる状況を必ず作る
		mu.Lock()
		defer mu.Unlock()
		ctxLive = ctx.Err() == nil
		tag = ctx.Value(ctxKey{})
	})
	cancel()
	close(started)

	waitForDelivery(t, runner)
	mu.Lock()
	defer mu.Unlock()
	if !ctxLive {
		t.Error("delivery ran with a cancelled context; the request's cancellation must not reach it")
	}
	if tag != "claims" {
		t.Errorf("context value = %v, want the request's value to survive", tag)
	}
}

// goroutine の panic はプロセスごと落とす。配信の失敗でサーバを落とさないこと、
// かつ後続の配信が止まらないこと。
func TestRunner_RecoversFromAPanicAndKeepsGoing(t *testing.T) {
	runner := NewRunner("test")

	delivered := make(chan struct{}, 1)
	runner.Go(context.Background(), "message_sent", func(context.Context) {
		panic("delivery blew up")
	})
	runner.Go(context.Background(), "room_marked_as_read", func(context.Context) {
		delivered <- struct{}{}
	})

	waitForDelivery(t, runner)
	select {
	case <-delivered:
	default:
		t.Fatal("the delivery after the panicking one never ran")
	}
}

// 配信同士は互いを待たない（重い授業ルームの宛先解決が他のルームの配信を止めない）。
func TestRunner_DeliveriesDoNotBlockEachOther(t *testing.T) {
	runner := NewRunner("test")

	gate := make(chan struct{})
	free := make(chan struct{}, 1)
	runner.Go(context.Background(), "message_sent", func(context.Context) { <-gate })
	runner.Go(context.Background(), "message_sent", func(context.Context) { free <- struct{}{} })

	select {
	case <-free:
	case <-time.After(5 * time.Second):
		t.Fatal("a delivery was blocked by another stalled one")
	}

	close(gate)
	waitForDelivery(t, runner)
}

// Wait は「片付くまで待つ」のであって「止める」ものではない。ctx が先に切れたら
// 待つのをやめて理由を返す（停止時の待ち合わせに上限を掛けられるように）。
func TestRunner_WaitGivesUpWhenItsContextExpires(t *testing.T) {
	runner := NewRunner("test")

	gate := make(chan struct{})
	runner.Go(context.Background(), "message_sent", func(context.Context) { <-gate })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := runner.Wait(ctx); err == nil {
		t.Error("Wait returned nil while a delivery was still running; it must report why it gave up")
	}

	close(gate)
	waitForDelivery(t, runner)
}

func TestRunner_RejectsWorkWhenAtCapacity(t *testing.T) {
	runner := NewRunnerWithLimits("test", 1, time.Second)
	gate := make(chan struct{})
	if !runner.TryGo(context.Background(), "blocking", func(context.Context) { <-gate }) {
		t.Fatal("first task must be accepted")
	}

	run := make(chan struct{}, 1)
	if runner.TryGo(context.Background(), "rejected", func(context.Context) { run <- struct{}{} }) {
		t.Fatal("task over capacity must report rejection")
	}
	select {
	case <-run:
		t.Fatal("the task over capacity must be rejected")
	case <-time.After(20 * time.Millisecond):
	}

	close(gate)
	waitForDelivery(t, runner)
}

func TestRunner_AddsATimeout(t *testing.T) {
	runner := NewRunnerWithLimits("test", 1, 20*time.Millisecond)
	errSeen := make(chan error, 1)
	runner.Go(context.Background(), "timed_out", func(ctx context.Context) {
		<-ctx.Done()
		errSeen <- ctx.Err()
	})

	waitForDelivery(t, runner)
	if err := <-errSeen; err != context.DeadlineExceeded {
		t.Fatalf("task context error = %v, want deadline exceeded", err)
	}
}
