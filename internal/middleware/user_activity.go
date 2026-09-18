package middleware

import (
	"context"
	"sync"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/async"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/rs/zerolog"
)

// userActivityInterval は同じユーザーの活動記録を書き直す最短間隔。
//
// 5分なのは DB 側の条件と揃えるため。UpdateLastActiveAt の UPDATE は
// `last_active_at < ? (now-300)` を付けており、5分以内の更新はもともと0行だった。
// つまりこの間引きで落ちるのは「投げても何も変わらない書き込み」だけで、
// 記録される内容は変わらない（変わるのは DB へ行く回数だけ）。
const userActivityInterval = 5 * time.Minute

// userActivitySweepInterval は覚えている「最後に書いた時刻」を掃除する間隔。
//
// 記録先は userID をキーにしたマップなので、掃除しないとプロセスが生きている間
// 「一度でもアクセスしたユーザー」の数だけ増え続ける（SSE の履歴と同じ壊れ方）。
// 掃除は記録のついでに行い、専用の goroutine は作らない（止め忘れを増やさない）。
const userActivitySweepInterval = 30 * time.Minute

// activityMark は「そのユーザーについて最後に活動を書いた時刻と、その時の JST 日付」。
//
// 日付を持つのは、間引きが日付をまたいでしまうのを防ぐため。
// 23:58 に書いた直後の 00:01 のアクセスは「5分以内」だが、活動日
// （user_activity_dates）としては新しい行が要る。日付が変わっていれば必ず書く。
type activityMark struct {
	at   time.Time
	date string
}

// UserActivityRecorder は認証済みリクエストの「最終アクセス時刻」と「活動日」を記録する。
//
// 以前は認証ミドルウェアが毎リクエスト、素の goroutine で UPDATE と INSERT を
// 撃っていた（しかも戻り値は `_ =` で捨てていた）。人が1画面開くだけで GraphQL や
// 画像の取得が何本も飛ぶので、書き込みの大半は DB 側の条件で0行になる無駄撃ちだった。
// ここで「同じユーザーについては userActivityInterval に1回だけ」に間引く。
//
// プロセスごとの記憶なので、多重起動すると台数ぶんだけ書き込みが起きる。それでも
// 毎リクエストよりは桁違いに少なく、DB 側の条件が最後の砦として残っている。
type UserActivityRecorder struct {
	users repository.UserRepository
	async *async.Runner

	// now と interval はテスト用に差し替えられるようにしてある（時間を待たずに
	// 間引きの境界を確かめるため）。本番は NewUserActivityRecorder が既定値を入れる。
	now      func() time.Time
	interval time.Duration

	mu        sync.Mutex
	last      map[int64]activityMark
	lastSweep time.Time
}

func NewUserActivityRecorder(users repository.UserRepository, asyncRunner *async.Runner) *UserActivityRecorder {
	return &UserActivityRecorder{
		users:    users,
		async:    asyncRunner,
		now:      time.Now,
		interval: userActivityInterval,
		last:     make(map[int64]activityMark),
	}
}

// jst は活動日の集計に使うタイムゾーン。tzdata が無い環境でも固定オフセットに
// 落ちるので、日付がずれることはあっても記録が止まることは無い。
var jst = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		return time.FixedZone("JST", 9*60*60)
	}
	return loc
}()

// Record はこのリクエストの活動を記録する（必要なときだけ）。
// 実際に書きに行ったかどうかを返す（テストと、呼び出し側で数える用途のため）。
func (r *UserActivityRecorder) Record(ctx context.Context, userID int64) bool {
	now := r.now()
	date := now.In(jst).Format("2006-01-02")

	r.mu.Lock()
	mark, seen := r.last[userID]
	// 前回の書き込みから interval 以内で、かつ日付も変わっていなければ書かない。
	if seen && mark.date == date && now.Sub(mark.at) < r.interval {
		r.mu.Unlock()
		return false
	}
	// 書く前に印を付ける。書いてから付けると、応答を返し終える前に同じユーザーの
	// 次のリクエストが来たときに二重で撃つ。失敗したぶんは次の interval まで
	// 記録されないが、それは「最終アクセス時刻が最大5分古い」だけの話で、
	// 失敗はログに残るので気づけなくなるわけではない。
	r.last[userID] = activityMark{at: now, date: date}
	r.sweepLocked(now)
	r.mu.Unlock()

	record := func(ctx context.Context) {
		if err := r.users.UpdateLastActiveAt(ctx, userID, now.Unix()); err != nil {
			logActivity(err).Int64("user_id", userID).Msg("failed to update last_active_at")
		}
		if err := r.users.LogActivityDate(ctx, userID, date); err != nil {
			logActivity(err).Int64("user_id", userID).Str("activity_date", date).Msg("failed to log the activity date")
		}
	}

	// 書き込みはリクエストの応答時間の外へ（記録が遅れても画面には関係ない）。
	// 投げっぱなしの流儀はサーバ全体で internal/async.Runner に揃えてあるので、
	// 停止時にはここも待ってもらえる（以前の素の goroutine は誰も待たなかった）。
	if r.async == nil {
		record(context.WithoutCancel(ctx))
		return true
	}
	r.async.Go(ctx, "user_activity", record)
	return true
}

// sweepLocked は古くなった印を捨てる。r.mu を持った状態で呼ぶこと。
//
// 「次に来ても必ず書き直す」ところまで古い印は、持っていても判定を変えないので
// 捨ててよい。全部なめるが、走るのは userActivitySweepInterval に1回だけ。
func (r *UserActivityRecorder) sweepLocked(now time.Time) {
	if now.Sub(r.lastSweep) < userActivitySweepInterval {
		return
	}
	r.lastSweep = now
	for userID, mark := range r.last {
		if now.Sub(mark.at) >= r.interval {
			delete(r.last, userID)
		}
	}
}

// logActivity は活動記録の失敗を必ず同じ形で残す。
// 以前は `_ =` で捨てていたので、DB が書けなくなっても誰も気づけなかった。
func logActivity(err error) *zerolog.Event {
	return logger.Log.Error().Err(err).Str("component", "user_activity")
}
