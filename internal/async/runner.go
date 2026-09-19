// Package async は「応答を待たせずに走らせる後始末」の唯一の実行口を提供する。
//
// 以前はこの手の処理が2通りあった。チャット配信は usecase/chat の AsyncRunner
// （ctx の引き継ぎ・panic の握り・停止時の待ち合わせを持つ）、お知らせ通知や
// 活動記録は素の `go func()` + context.Background()（待ち合わせ無し・panic で
// プロセス死・エラー握り潰し）。同じ「投げっぱなし」に2つの流儀があると、
// 停止時に待つ経路と待たない経路が混ざり、DB を閉じたあとに走る書き込みが
// 残り続ける。口はここ1つに揃える。
package async

import (
	"context"
	"sync"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
)

// Runner はリクエストの応答時間の外で関数を1回だけ走らせる。
//
// 保証するのは5つだけで、順序は保証しない（渡した関数は goroutine で即座に走る）。
//
//  1. ctx の値は保ち、キャンセルだけ外す（context.WithoutCancel）
//  2. panic を握ってログに残す（配信の取りこぼしでプロセスを落とさない）
//  3. 走っているものを Wait で待てる（停止時に DB を閉じる前に片付ける）
//  4. 同時実行は既定100件までとし、満杯なら待たせず拒否してログへ残す
//  5. 各処理の context は既定15秒で期限切れにする
//
// 順序が要る処理をここへ渡さないこと。何をここへ渡してよいかの判断は、
// 呼び出し側（usecase/chat/async_events.go のコメントが詳しい）が持つ。
type Runner struct {
	// component は失敗ログの component フィールド。どの経路の投げっぱなしが
	// 落ちたのかをログの絞り込みだけで追えるようにするため、Runner ごとに持つ。
	component string
	slots     chan struct{}
	timeout   time.Duration

	// wg は走っている処理の数。停止時の待ち合わせとテストの同期に使う。
	wg sync.WaitGroup
}

const (
	defaultMaxConcurrent = 100
	defaultTaskTimeout   = 15 * time.Second
)

// NewRunner は component 名を持つ Runner を返す。
// component は構造化ログの絞り込みに使う名前（例: "chat_event_publisher"）。
func NewRunner(component string) *Runner {
	return NewRunnerWithLimits(component, defaultMaxConcurrent, defaultTaskTimeout)
}

// NewRunnerWithLimits is exposed for deterministic boundary and timeout tests.
func NewRunnerWithLimits(component string, maxConcurrent int, timeout time.Duration) *Runner {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &Runner{
		component: component,
		slots:     make(chan struct{}, maxConcurrent),
		timeout:   timeout,
	}
}

// Go は fn をリクエストの外で1回実行する。name は失敗ログの識別子。
//
// ctx から切り離すのは「キャンセルだけ」（context.WithoutCancel）。リクエストの ctx を
// そのまま goroutine へ渡すと、レスポンスを返した時点でキャンセルされ、処理中の
// DB アクセスが context canceled で軒並み失敗する。かといって context.Background() に
// 差し替えると ctx に載っている値まで落ちてしまい、認証情報（internal/auth の Claims。
// 監査ログや権限判定が読む）やロガー・リクエストIDが消える。値は保ちキャンセルだけ
// 外す WithoutCancel が、ちょうど要るものになる。
func (r *Runner) Go(ctx context.Context, name string, fn func(context.Context)) {
	r.TryGo(ctx, name, fn)
}

// TryGo reports whether the bounded runner accepted the task.
func (r *Runner) TryGo(ctx context.Context, name string, fn func(context.Context)) bool {
	if fn == nil {
		return false
	}
	select {
	case r.slots <- struct{}{}:
	default:
		logger.Log.Warn().
			Str("component", r.component).
			Str("delivery", "async_rejected").
			Str("event", name).
			Int("active", len(r.slots)).
			Int("max_concurrent", cap(r.slots)).
			Msg("rejected an async task because the runner is full")
		return false
	}

	detached := context.WithoutCancel(ctx)
	cancel := func() {}
	if r.timeout > 0 {
		detached, cancel = context.WithTimeout(detached, r.timeout)
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer func() { <-r.slots }()
		defer cancel()
		defer func() {
			// goroutine の panic はプロセスごと落とす。同期実行だったころは
			// GraphQL のリカバリまで上がってリクエストがエラーになるだけで済んだ。
			// 取りこぼしでサーバを落とすのは割に合わないので必ず recover し、
			// 握り潰した事実が消えないようログには必ず出す。
			if p := recover(); p != nil {
				logger.Log.Error().
					Str("component", r.component).
					Str("delivery", "async_panic").
					Str("event", name).
					Interface("panic", p).
					Msg("recovered from a panic in an async task")
			}
		}()
		fn(detached)
		if detached.Err() == context.DeadlineExceeded {
			logger.Log.Warn().
				Str("component", r.component).
				Str("delivery", "async_timeout").
				Str("event", name).
				Dur("timeout", r.timeout).
				Msg("an async task reached its deadline")
		}
	}()
	return true
}

// Wait は走っている処理が終わるまで待つ。
//
// 用途は2つ。(1) 停止時に、レスポンスを返し終えたあとの取りこぼしを DB 接続を
// 閉じる前に減らす。(2) テストで「非同期だから確かめられない」を避ける。
// ctx が先に切れたら待つのをやめて ctx.Err() を返す（処理中のものは走り続ける）。
func (r *Runner) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
