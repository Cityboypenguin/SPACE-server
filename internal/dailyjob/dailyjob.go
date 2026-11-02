// Package dailyjob は「起動時に1回、その後は毎日決まった時刻（JST）」に走らせる処理の
// 土台。何をするかは Exec に任せ、ここはいつ走らせるかと失敗時のやり直しだけを持つ。
package dailyjob

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
)

var jst = time.FixedZone("Asia/Tokyo", 9*60*60)

const (
	// retryInterval は失敗したときに次を試すまでの間隔。
	retryInterval = time.Hour
	// runTimeout は1回の実行の上限。
	runTimeout = 30 * time.Minute
)

// Runner は1つの日次処理。
type Runner struct {
	// Name はログの component。
	Name string
	// HourJST は日次で走らせる時刻。利用の少ない時間帯にする。
	HourJST int
	// Exec は1回ぶんの処理。処理した件数を返す。
	Exec func(ctx context.Context, now time.Time) (int, error)
	// DoneMessage は1件以上処理したときのログの文言。
	DoneMessage string
	// Now は時刻の取得元。nil なら time.Now。
	Now func() time.Time
}

// Run は ctx が終わるまで、起動時に1回、その後は毎日 HourJST 時に実行する。
func (r *Runner) Run(ctx context.Context) {
	now := r.Now
	if now == nil {
		now = time.Now
	}
	for {
		failed := r.runAndLog(ctx, now())
		if ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(NextRunDelay(now(), r.HourJST, failed))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// NextRunDelay は次の実行までの待ち時間。失敗したら1時間後、成功したら次の hourJST 時。
func NextRunDelay(now time.Time, hourJST int, failed bool) time.Duration {
	if failed {
		return retryInterval
	}
	local := now.In(jst)
	next := time.Date(local.Year(), local.Month(), local.Day(), hourJST, 0, 0, 0, jst)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Sub(local)
}

// runAndLog は1回実行し、失敗したかを返す。
func (r *Runner) runAndLog(parent context.Context, now time.Time) bool {
	ctx, cancel := context.WithTimeout(parent, runTimeout)
	defer cancel()

	done, err := r.Exec(ctx, now)
	if done > 0 {
		logger.Log.Info().Str("component", r.Name).Int("count", done).Msg(r.DoneMessage)
	}
	if err == nil || parent.Err() != nil {
		return false
	}
	logger.Log.Error().Err(err).Str("component", r.Name).Msg("daily job failed; retrying")
	return true
}
