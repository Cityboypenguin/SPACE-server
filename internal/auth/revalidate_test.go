package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// shortenRevalidate はテストのあいだだけ確かめ直しの間隔を縮める。
func shortenRevalidate(t *testing.T, d time.Duration) {
	t.Helper()
	prev := SessionRevalidateInterval
	SessionRevalidateInterval = d
	t.Cleanup(func() { SessionRevalidateInterval = prev })
}

func waitDone(t *testing.T, ctx context.Context, within time.Duration) bool {
	t.Helper()
	select {
	case <-ctx.Done():
		return true
	case <-time.After(within):
		return false
	}
}

// TestWatchSession_EndsTheConnectionOnceTheTokenStopsWorking は今回の本体。
//
// WebSocket と SSE は接続を張るときに1度しか認証を通らない。何も足さないと、
// 管理者を消しても、利用者を凍結しても、ログアウトしても、開いたままのタブには
// 新着が流れ続ける（切るにはサーバーの再起動しかない）。
func TestWatchSession_EndsTheConnectionOnceTheTokenStopsWorking(t *testing.T) {
	shortenRevalidate(t, time.Millisecond)

	var mu sync.Mutex
	valid := true
	ctx := WatchSession(context.Background(), func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		if valid {
			return nil
		}
		return errors.New("token has been revoked")
	}, nil)

	// 通用しているうちは切れない。
	if waitDone(t, ctx, 20*time.Millisecond) {
		t.Fatal("まだ通用するのに接続が切られた")
	}

	mu.Lock()
	valid = false
	mu.Unlock()

	if !waitDone(t, ctx, 2*time.Second) {
		t.Fatal("失効しても接続が切れない")
	}
}

// TestWatchSession_SurvivesTransientVerificationFailures は、確かめられなかった
// だけで切らないことを確かめる。
//
// 切ってしまうと、Redis や DB の一瞬の不調で全ての WebSocket と SSE が同時に
// 切れ、その全部が一斉に張り直しに来る。不調をこちらで増幅することになる。
func TestWatchSession_SurvivesTransientVerificationFailures(t *testing.T) {
	shortenRevalidate(t, time.Millisecond)

	var mu sync.Mutex
	calls := 0
	ctx := WatchSession(context.Background(), func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		// 最初の数回だけ確かめられない。その後は通る。
		if calls <= 3 {
			return fmt.Errorf("failed to verify token: %w", ErrVerificationUnavailable)
		}
		return nil
	}, nil)

	if waitDone(t, ctx, 50*time.Millisecond) {
		t.Fatal("一時的に確かめられなかっただけで接続が切られた")
	}
}

// TestWatchSession_GivesUpWhenItCanNeverVerify は、見送り続けないことを確かめる。
//
// 無制限に見送ると「置き場が落ちている間は失効が効かない」になる。
func TestWatchSession_GivesUpWhenItCanNeverVerify(t *testing.T) {
	shortenRevalidate(t, time.Millisecond)

	ctx := WatchSession(context.Background(), func(context.Context) error {
		return fmt.Errorf("failed to verify token: %w", ErrVerificationUnavailable)
	}, nil)

	if !waitDone(t, ctx, 2*time.Second) {
		t.Fatal("ずっと確かめられないのに接続が残り続けている")
	}
}

// TestWatchSession_StopsWatchingWhenTheConnectionCloses は、接続が閉じたら
// 見張りも消えることを確かめる。残ると、切れた接続の数だけ認証の問い合わせが
// 走り続ける。
func TestWatchSession_StopsWatchingWhenTheConnectionCloses(t *testing.T) {
	shortenRevalidate(t, time.Millisecond)

	parent, cancel := context.WithCancel(context.Background())

	var mu sync.Mutex
	calls := 0
	ctx := WatchSession(parent, func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return nil
	}, nil)

	time.Sleep(10 * time.Millisecond)
	cancel()
	if !waitDone(t, ctx, time.Second) {
		t.Fatal("親が終わったのに派生した ctx が終わっていない")
	}

	// 止まったことを、呼び出し回数が増えなくなることで見る。
	time.Sleep(10 * time.Millisecond)
	mu.Lock()
	settled := calls
	mu.Unlock()
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if calls != settled {
		t.Fatalf("見張りが残っている（%d -> %d）", settled, calls)
	}
}

// TestWatchSession_ReportsWhyItClosed は、切った理由が呼び出し側へ渡ること。
// ログに残らないと、「繋がらない」という問い合わせの原因を追えない。
func TestWatchSession_ReportsWhyItClosed(t *testing.T) {
	shortenRevalidate(t, time.Millisecond)

	reported := make(chan error, 1)
	ctx := WatchSession(context.Background(), func(context.Context) error {
		return errors.New("account is frozen")
	}, func(err error) { reported <- err })

	if !waitDone(t, ctx, 2*time.Second) {
		t.Fatal("接続が切れない")
	}
	select {
	case err := <-reported:
		if err == nil || err.Error() != "account is frozen" {
			t.Fatalf("reported = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("切った理由が渡ってこない")
	}
}

// 確かめ直しを配線しない経路（テストや、認証を組み立てない起動）では
// 素通しすること。
func TestWatchSession_WithoutACheckerIsATransparentPassThrough(t *testing.T) {
	parent := context.Background()
	if got := WatchSession(parent, nil, nil); got != parent {
		t.Fatal("checker が無いなら ctx はそのまま返すこと")
	}
}
