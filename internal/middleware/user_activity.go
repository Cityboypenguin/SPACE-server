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
//
// 活動日・活動時間帯の INSERT も同じ性質で、同じ日・同じ時間帯の2回目以降は
// INSERT IGNORE が捨てる。だから「単位をまたいだら間引かない」（activityMark）
// さえ守れば、間引きで記録が欠けることはない。
const userActivityInterval = 5 * time.Minute

// userActivitySweepInterval は覚えている「最後に書いた時刻」を掃除する間隔。
//
// 記録先は userID をキーにしたマップなので、掃除しないとプロセスが生きている間
// 「一度でもアクセスしたユーザー」の数だけ増え続ける（SSE の履歴と同じ壊れ方）。
// 掃除は記録のついでに行い、専用の goroutine は作らない（止め忘れを増やさない）。
const userActivitySweepInterval = 30 * time.Minute

// activityMark は「そのユーザーについて最後に活動を書いた時刻と、その時の JST 時間帯」。
//
// 時間帯を持つのは、間引きが記録の単位をまたいでしまうのを防ぐため。
// 10:58 に書いた直後の 11:02 のアクセスは「5分以内」だが、活動した時間帯
// （user_activity_hours）としては新しい行が要る。間引いてしまうと11時台の
// スロットが丸ごと落ち、時間別グラフがまた実際より少なく出る。
//
// 以前は JST の日付を持っていて「日付が変わったら必ず書く」だけだった。
// 時間帯の文字列は日付を含む（activityHourFormat）ので、日付が変われば時間帯も
// 必ず変わる。つまり時間帯で見る新しい規則は、前の規則をそのまま含んでいる。
// 日付の境目の扱いは変わっていない。
type activityMark struct {
	at   time.Time
	hour string
}

// 記録の単位。activityDateFormat は JST の日付（user_activity_dates）、
// activityHourFormat は JST の時の始まり（user_activity_hours）。
// どちらも DB の列（DATE / DATETIME）へそのまま渡せる形にしてある。
const (
	activityDateFormat = "2006-01-02"
	activityHourFormat = "2006-01-02 15:00:00"
)

// UserActivityRecorder は認証済みリクエストの「最終アクセス時刻」と「活動日」と
// 「活動した時間帯」を記録する。
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

// jst は活動日・活動時間帯の集計に使うタイムゾーン。tzdata が無い環境でも固定オフセットに
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
	jstNow := now.In(jst)
	date := jstNow.Format(activityDateFormat)
	hour := jstNow.Format(activityHourFormat)

	r.mu.Lock()
	mark, seen := r.last[userID]
	// 前回の書き込みから interval 以内で、かつ時間帯も変わっていなければ書かない。
	// 時間帯が変わっていれば（日付が変わったときも必ずそうなる）必ず書く。
	if seen && mark.hour == hour && now.Sub(mark.at) < r.interval {
		r.mu.Unlock()
		return false
	}
	record := func(ctx context.Context) {
		if err := r.users.UpdateLastActiveAt(ctx, userID, now.Unix()); err != nil {
			logActivity(err).Int64("user_id", userID).Msg("failed to update last_active_at")
		}
		if err := r.users.LogActivityDate(ctx, userID, date); err != nil {
			logActivity(err).Int64("user_id", userID).Str("activity_date", date).Msg("failed to log the activity date")
		}
		// 活動日と活動時間帯は別々に書く（片方が失敗しても片方は残る）。
		// 日次の集計は前者、時間別の集計は後者だけを読むので、取りこぼしは
		// その系列のその1点に閉じる。
		if err := r.users.LogActivityHour(ctx, userID, hour); err != nil {
			logActivity(err).Int64("user_id", userID).Str("activity_hour", hour).Msg("failed to log the activity hour")
		}
	}

	// 書き込みはリクエストの応答時間の外へ（記録が遅れても画面には関係ない）。
	// 投げっぱなしの流儀はサーバ全体で internal/async.Runner に揃えてあるので、
	// 停止時にはここも待ってもらえる（以前の素の goroutine は誰も待たなかった）。
	if r.async == nil {
		r.last[userID] = activityMark{at: now, hour: hour}
		r.sweepLocked(now)
		r.mu.Unlock()
		record(context.WithoutCancel(ctx))
		return true
	}
	// TryGo is nonblocking: keep the mark lock until the runner has accepted the task.
	// A rejected task must remain eligible on the next request.
	if !r.async.TryGo(ctx, "user_activity", record) {
		r.mu.Unlock()
		return false
	}
	r.last[userID] = activityMark{at: now, hour: hour}
	r.sweepLocked(now)
	r.mu.Unlock()
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
