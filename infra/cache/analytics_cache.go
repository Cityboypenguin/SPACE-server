package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/metrics"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"golang.org/x/sync/singleflight"
)

// CachedAnalyticsRepository wraps AnalyticsRepository with in-memory TTL caching.
// GetAnalyticsSummary is cached for summaryTTL (expensive: 35+ DB queries).
// GetTimeSeries and GetCommunityAnalytics are cached for keyedTTL.
//
// 3つとも「キー付きの TTL キャッシュ + singleflight + 入れるついでの掃除」で
// 同じ形にしてある。キーには必ず「何を計算したか」を含める（下記）。
// ■ 期限切れ直後の重複集計を防ぐ（singleflight）
//
// キャッシュは「無ければ集計して入れる」だけなので、期限が切れた瞬間に同時に
// 来たリクエストは全員がミスし、全員が同じ集計（サマリーなら 35 本以上の SQL）
// を走らせていた。管理画面を複数人で開いている・タブが自動更新する、といった
// ごく普通の状況で起きる。
//
// singleflight は「同じキーの取得は1本だけ走らせ、残りはその結果を待つ」ので、
// 重複ぶんがそのまま消える。キーはキャッシュのキーと同じものを使うこと
// （別物にすると、違う条件の取得が1本にまとめられて取り違えになる）。
//
// 注意: 集計を実際に走らせるのは「最初に来た1人」の ctx。その人が離脱すると
// 待っている側もその ctx のキャンセルを受け取る。管理画面の再読み込みで済む
// 範囲なので、ここでは ctx を付け替えていない。

type CachedAnalyticsRepository struct {
	inner repository.AnalyticsRepository

	// 3経路それぞれに group を分ける。1つの group を接頭辞付きキーで共有すると、
	// 接頭辞を付け忘れたときに戻り値の型が食い違って実行時に落ちる。
	sfSummary singleflight.Group
	sfTS      singleflight.Group
	sfComm    singleflight.Group

	// GetAnalyticsSummary — keyed by FieldSet.CacheKey()
	//
	// 以前は引数が無く単一スロットだった。要求されたフィールドだけ集計する
	// ようになったので、「一部しか計算していない結果」を全部要求した側へ
	// 配らないよう、何を計算したかをキーにする（PageQuery.WithTotal を
	// キーへ足したのと同じ理由。あちらが total ひとつだったのに対し、
	// こちらはフィールドの数だけ同型の取り違えが起きうる）。
	summaryMu        sync.RWMutex
	summaryEntries   map[string]summaryEntry
	summaryTTL       time.Duration
	summaryLastSweep time.Time

	// GetTimeSeries — keyed by "granularity:from:to:series"
	tsMu        sync.RWMutex
	tsEntries   map[string]tsEntry
	tsTTL       time.Duration
	tsLastSweep time.Time

	// GetCommunityAnalytics — keyed by "limit:offset:withTotal"
	commMu        sync.RWMutex
	commEntries   map[string]commEntry
	commTTL       time.Duration
	commLastSweep time.Time

	// now はテストで時間を進めるための差し替え口（期限切れを待たずに試すため）。
	// internal/sse の Broker と同じ形にしてある。
	now func() time.Time
}

// cacheSweepInterval は期限切れキーの掃除を走らせる最短間隔。
//
// 掃除は「新しい結果を入れるついで」に行い、専用の goroutine は持たない
// （internal/sse/broker.go の履歴掃除と同じ考え方。止め忘れた goroutine の
// ほうが漏れとしては厄介なので、停止処理の要らない形に揃える）。掃除は
// キーを全てなめるが、走るのはこの間隔に1回だけ。
//
// 掃除が要るのは、時系列とコミュニティ一覧のキャッシュがキー付きだから。
// キーは「粒度 × 期間の組み合わせ」「limit × offset × total の有無」で、
// 管理画面で期間を動かすたびに新しいキーが増える。以前は期限が切れた
// エントリも消していなかったので、触られた組み合わせの数だけメモリが
// 増え続けていた（サマリーは単一スロットなのでこの問題が無い）。
const cacheSweepInterval = time.Minute

// sweepExpired は期限切れのエントリを捨てる。対象の mutex を書き込みで
// 持った状態で呼ぶこと。lastSweep を進めるので、最短間隔の判定も兼ねる。
func sweepExpired[V any](entries map[string]V, lastSweep *time.Time, now time.Time, expiresOf func(V) time.Time) {
	if now.Sub(*lastSweep) < cacheSweepInterval {
		return
	}
	*lastSweep = now
	for k, e := range entries {
		if !now.Before(expiresOf(e)) {
			delete(entries, k)
		}
	}
}

type summaryEntry struct {
	val     *model.AnalyticsSummary
	expires time.Time
}

type tsEntry struct {
	val     []*model.TimeSeriesPoint
	expires time.Time
}

type commEntry struct {
	val     []*model.CommunityStatItem
	total   int
	expires time.Time
}

// InvalidateSummary はサマリーキャッシュを即座に破棄する。
// 通報ステータス変更など管理操作の直後に呼ぶことで、次回取得時にDBから最新値を読ませる。
//
// フィールドの組み合わせごとにエントリがあるので、1つ残らず捨てる（管理操作で
// 変わった数字が、別の組み合わせのキーに古いまま残っていては意味が無い）。
func (c *CachedAnalyticsRepository) InvalidateSummary() {
	c.summaryMu.Lock()
	clear(c.summaryEntries)
	c.summaryMu.Unlock()
}

func NewCachedAnalyticsRepository(
	inner repository.AnalyticsRepository,
	summaryTTL, keyedTTL time.Duration,
) *CachedAnalyticsRepository {
	return &CachedAnalyticsRepository{
		inner:          inner,
		summaryTTL:     summaryTTL,
		summaryEntries: make(map[string]summaryEntry),
		tsEntries:      make(map[string]tsEntry),
		tsTTL:          keyedTTL,
		commEntries:    make(map[string]commEntry),
		commTTL:        keyedTTL,
		now:            time.Now,
	}
}

func (c *CachedAnalyticsRepository) GetAnalyticsSummary(ctx context.Context, fields repository.FieldSet) (*model.AnalyticsSummary, error) {
	key := fields.CacheKey()

	c.summaryMu.RLock()
	if e, ok := c.summaryEntries[key]; ok && c.now().Before(e.expires) {
		// DB由来フィールドはキャッシュのまま、in-memoryメトリクスだけ毎回最新値で上書き
		snapshot := *e.val
		c.summaryMu.RUnlock()
		metrics.ApplyToSummary(&snapshot)
		return &snapshot, nil
	}
	c.summaryMu.RUnlock()

	v, err, _ := c.sfSummary.Do(key, func() (any, error) {
		val, err := c.inner.GetAnalyticsSummary(ctx, fields)
		if err != nil {
			return nil, err
		}
		now := c.now()
		c.summaryMu.Lock()
		c.summaryEntries[key] = summaryEntry{val: val, expires: now.Add(c.summaryTTL)}
		sweepExpired(c.summaryEntries, &c.summaryLastSweep, now, func(e summaryEntry) time.Time { return e.expires })
		c.summaryMu.Unlock()
		return val, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*model.AnalyticsSummary), nil
}

func (c *CachedAnalyticsRepository) GetTimeSeries(ctx context.Context, granularity, from, to string, series repository.FieldSet) ([]*model.TimeSeriesPoint, error) {
	// series をキーに含めるのを外さないこと。含めないと「posts しか引かなかった
	// 応答（他の系列は 0）」が同じ窓のキーに載り、あとから messages を選んだ
	// リクエストへ 0 を返してしまう。
	key := fmt.Sprintf("%s:%s:%s:%s", granularity, from, to, series.CacheKey())

	c.tsMu.RLock()
	if e, ok := c.tsEntries[key]; ok && c.now().Before(e.expires) {
		v := e.val
		c.tsMu.RUnlock()
		return v, nil
	}
	c.tsMu.RUnlock()

	v, err, _ := c.sfTS.Do(key, func() (any, error) {
		val, err := c.inner.GetTimeSeries(ctx, granularity, from, to, series)
		if err != nil {
			return nil, err
		}
		now := c.now()
		c.tsMu.Lock()
		c.tsEntries[key] = tsEntry{val: val, expires: now.Add(c.tsTTL)}
		// 入れるついでに期限切れを捨てる（最短で cacheSweepInterval に1回）。
		sweepExpired(c.tsEntries, &c.tsLastSweep, now, func(e tsEntry) time.Time { return e.expires })
		c.tsMu.Unlock()
		return val, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]*model.TimeSeriesPoint), nil
}

func (c *CachedAnalyticsRepository) GetCommunityAnalytics(ctx context.Context, q repository.PageQuery) ([]*model.CommunityStatItem, int, error) {
	// WithTotal をキーに含めるのを外さないこと。含めないと「total を数えなかった
	// 応答（total=0）」が同じ窓のキーに載り、あとから total を選んだリクエストへ
	// 0 を返してしまう。窓が同じでも中身が違うので別のキーになる。
	key := fmt.Sprintf("%d:%d:%t", q.Limit, q.Offset, q.WithTotal)

	c.commMu.RLock()
	if e, ok := c.commEntries[key]; ok && c.now().Before(e.expires) {
		v, t := e.val, e.total
		c.commMu.RUnlock()
		return v, t, nil
	}
	c.commMu.RUnlock()

	v, err, _ := c.sfComm.Do(key, func() (any, error) {
		val, total, err := c.inner.GetCommunityAnalytics(ctx, q)
		if err != nil {
			return nil, err
		}
		now := c.now()
		e := commEntry{val: val, total: total, expires: now.Add(c.commTTL)}
		c.commMu.Lock()
		c.commEntries[key] = e
		sweepExpired(c.commEntries, &c.commLastSweep, now, func(e commEntry) time.Time { return e.expires })
		c.commMu.Unlock()
		return e, nil
	})
	if err != nil {
		return nil, 0, err
	}
	e := v.(commEntry)
	return e.val, e.total, nil
}
