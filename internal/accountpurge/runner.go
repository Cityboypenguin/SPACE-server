// Package accountpurge は、退会の猶予が切れた利用者の個人情報を毎日消す。
//
// 消す手順そのものは usecase/user.PurgeExpiredAccountsUseCase にある。ここは
// 「いつ走らせるか」だけを持つ（起動時に1回、その後は毎日 JST 4時）。
//
// 複数のサーバーで同時に走っても壊れない。消す直前に利用者の行をロックして
// 状態を確かめ直すので、同じ人を二重に消すことはなく、後から来た側は何もしない。
package accountpurge

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	userusecase "github.com/Cityboypenguin/SPACE-server/usecase/user"
)

var jst = time.FixedZone("Asia/Tokyo", 9*60*60)

const (
	// runHourJST は日次で走らせる時刻。利用の少ない時間帯にする。
	runHourJST = 4
	// retryInterval は失敗したときに次を試すまでの間隔。
	retryInterval = time.Hour
	// runTimeout は1回の実行の上限。
	runTimeout = 30 * time.Minute
)

type Runner struct {
	purge userusecase.PurgeExpiredAccountsUseCase
	now   func() time.Time
}

func New(purge userusecase.PurgeExpiredAccountsUseCase) *Runner {
	return &Runner{purge: purge, now: time.Now}
}

// Run は ctx が終わるまで、起動時に1回、その後は毎日 JST 4時に実行する。
func (r *Runner) Run(ctx context.Context) {
	for {
		failed := r.runAndLog(ctx)
		if ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(nextRunDelay(r.now(), failed))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// nextRunDelay は次の実行までの待ち時間。失敗したら1時間後、成功したら次の JST 4時。
func nextRunDelay(now time.Time, failed bool) time.Duration {
	if failed {
		return retryInterval
	}
	local := now.In(jst)
	next := time.Date(local.Year(), local.Month(), local.Day(), runHourJST, 0, 0, 0, jst)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Sub(local)
}

// runAndLog は1回実行し、失敗したかを返す。
func (r *Runner) runAndLog(parent context.Context) bool {
	ctx, cancel := context.WithTimeout(parent, runTimeout)
	defer cancel()

	purged, err := r.purge.Execute(ctx, r.now())
	if purged > 0 {
		logger.Log.Info().Str("component", "account_purge").Int("purged", purged).
			Msg("purged the personal data of users whose grace period expired")
	}
	if err == nil || parent.Err() != nil {
		return false
	}
	logger.Log.Error().Err(err).Str("component", "account_purge").Msg("account purge run failed; retrying")
	return true
}
