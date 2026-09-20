package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// fakeAnalyticsRepo は「何回呼ばれたか」を数え、必要なら呼び出しを足止めできる
// 内側のリポジトリ。singleflight が効いているかは回数でしか分からない。
type fakeAnalyticsRepo struct {
	summaryCalls atomic.Int64
	tsCalls      atomic.Int64
	commCalls    atomic.Int64

	// entered は集計に入ったことを知らせる。release を閉じるまで集計は返らない
	// （両方 nil なら足止めしない）。
	entered chan struct{}
	release chan struct{}

	err error

	// lastSummaryFields は最後に渡された要求フィールド。キャッシュが
	// 別の組み合わせの結果を配っていないかを確かめるのに使う。
	lastSummaryFields repository.FieldSet
}

func (f *fakeAnalyticsRepo) block() {
	if f.entered != nil {
		f.entered <- struct{}{}
	}
	if f.release != nil {
		<-f.release
	}
}

func (f *fakeAnalyticsRepo) GetAnalyticsSummary(ctx context.Context, fields repository.FieldSet) (*model.AnalyticsSummary, error) {
	f.summaryCalls.Add(1)
	f.lastSummaryFields = fields
	f.block()
	if f.err != nil {
		return nil, f.err
	}
	return &model.AnalyticsSummary{TotalUsers: 7}, nil
}

func (f *fakeAnalyticsRepo) GetTimeSeries(ctx context.Context, granularity, from, to string, series repository.FieldSet) ([]*model.TimeSeriesPoint, error) {
	f.tsCalls.Add(1)
	f.block()
	if f.err != nil {
		return nil, f.err
	}
	return []*model.TimeSeriesPoint{{Label: from + ".." + to + ":" + granularity}}, nil
}

func (f *fakeAnalyticsRepo) GetCommunityAnalytics(ctx context.Context, q repository.PageQuery) ([]*model.CommunityStatItem, int, error) {
	f.commCalls.Add(1)
	f.block()
	if f.err != nil {
		return nil, 0, f.err
	}
	return []*model.CommunityStatItem{{Name: "c"}}, q.Offset + 1, nil
}

var _ repository.AnalyticsRepository = (*fakeAnalyticsRepo)(nil)

const concurrentCallers = 16

// 足止め中の集計へ後続が合流し、内側が1回しか走らないこと。
// 「全員が中に入ってから放流する」ことで、たまたま直列に流れて1回に見える形を
// 避けている。
func runConcurrently(t *testing.T, n int, fn func()) {
	t.Helper()
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			fn()
		}()
	}
	wg.Wait()
}

func TestSummarySingleflight(t *testing.T) {
	inner := &fakeAnalyticsRepo{entered: make(chan struct{}, concurrentCallers), release: make(chan struct{})}
	c := NewCachedAnalyticsRepository(inner, time.Minute, time.Minute)

	started := make(chan struct{}, concurrentCallers)
	done := make(chan struct{})
	go func() {
		runConcurrently(t, concurrentCallers, func() {
			started <- struct{}{}
			if _, err := c.GetAnalyticsSummary(context.Background(), repository.AllFields()); err != nil {
				t.Errorf("GetAnalyticsSummary failed: %v", err)
			}
		})
		close(done)
	}()

	// 先頭が集計に入ったことを確認してから、残り全員がメソッドに入るのを待つ。
	<-inner.entered
	for i := 0; i < concurrentCallers; i++ {
		<-started
	}
	// メソッドに入ってから singleflight に合流するまでの隙間を埋める。
	time.Sleep(50 * time.Millisecond)
	close(inner.release)
	<-done

	if got := inner.summaryCalls.Load(); got != 1 {
		t.Fatalf("inner GetAnalyticsSummary ran %d times, want 1", got)
	}
}

func TestTimeSeriesSingleflight(t *testing.T) {
	inner := &fakeAnalyticsRepo{entered: make(chan struct{}, concurrentCallers), release: make(chan struct{})}
	c := NewCachedAnalyticsRepository(inner, time.Minute, time.Minute)

	started := make(chan struct{}, concurrentCallers)
	done := make(chan struct{})
	go func() {
		runConcurrently(t, concurrentCallers, func() {
			started <- struct{}{}
			if _, err := c.GetTimeSeries(context.Background(), "day", "2026-01-01", "2026-01-31", repository.AllFields()); err != nil {
				t.Errorf("GetTimeSeries failed: %v", err)
			}
		})
		close(done)
	}()

	<-inner.entered
	for i := 0; i < concurrentCallers; i++ {
		<-started
	}
	time.Sleep(50 * time.Millisecond)
	close(inner.release)
	<-done

	if got := inner.tsCalls.Load(); got != 1 {
		t.Fatalf("inner GetTimeSeries ran %d times, want 1", got)
	}
}

func TestCommunityAnalyticsSingleflight(t *testing.T) {
	inner := &fakeAnalyticsRepo{entered: make(chan struct{}, concurrentCallers), release: make(chan struct{})}
	c := NewCachedAnalyticsRepository(inner, time.Minute, time.Minute)

	q := repository.PageQuery{Limit: 20, Offset: 0, WithTotal: true}
	started := make(chan struct{}, concurrentCallers)
	done := make(chan struct{})
	go func() {
		runConcurrently(t, concurrentCallers, func() {
			started <- struct{}{}
			items, total, err := c.GetCommunityAnalytics(context.Background(), q)
			if err != nil {
				t.Errorf("GetCommunityAnalytics failed: %v", err)
				return
			}
			if len(items) != 1 || total != 1 {
				t.Errorf("GetCommunityAnalytics = %d items / total %d, want 1 / 1", len(items), total)
			}
		})
		close(done)
	}()

	<-inner.entered
	for i := 0; i < concurrentCallers; i++ {
		<-started
	}
	time.Sleep(50 * time.Millisecond)
	close(inner.release)
	<-done

	if got := inner.commCalls.Load(); got != 1 {
		t.Fatalf("inner GetCommunityAnalytics ran %d times, want 1", got)
	}
}

// 条件が違う取得は合流させない（合流させると別条件の結果を配ってしまう）。
func TestSingleflightKeepsDifferentKeysApart(t *testing.T) {
	inner := &fakeAnalyticsRepo{}
	c := NewCachedAnalyticsRepository(inner, time.Minute, time.Minute)
	ctx := context.Background()

	a, err := c.GetTimeSeries(ctx, "day", "2026-01-01", "2026-01-31", repository.AllFields())
	if err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	b, err := c.GetTimeSeries(ctx, "hour", "2026-02-01", "2026-02-02", repository.AllFields())
	if err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	if a[0].Label == b[0].Label {
		t.Fatalf("different windows shared a result: %q", a[0].Label)
	}
	if got := inner.tsCalls.Load(); got != 2 {
		t.Fatalf("inner GetTimeSeries ran %d times, want 2", got)
	}

	// PageQuery も同じ。WithTotal 違いは別のキー。
	if _, _, err := c.GetCommunityAnalytics(ctx, repository.PageQuery{Limit: 20, WithTotal: false}); err != nil {
		t.Fatalf("GetCommunityAnalytics failed: %v", err)
	}
	if _, _, err := c.GetCommunityAnalytics(ctx, repository.PageQuery{Limit: 20, WithTotal: true}); err != nil {
		t.Fatalf("GetCommunityAnalytics failed: %v", err)
	}
	if got := inner.commCalls.Load(); got != 2 {
		t.Fatalf("inner GetCommunityAnalytics ran %d times, want 2 (WithTotal 違いは別キー)", got)
	}
}

// 失敗はキャッシュしない。次の取得でやり直せること。
func TestSingleflightDoesNotCacheErrors(t *testing.T) {
	inner := &fakeAnalyticsRepo{err: errors.New("boom")}
	c := NewCachedAnalyticsRepository(inner, time.Minute, time.Minute)
	ctx := context.Background()

	if _, err := c.GetAnalyticsSummary(ctx, repository.AllFields()); err == nil {
		t.Fatal("GetAnalyticsSummary succeeded, want the inner error")
	}
	inner.err = nil
	s, err := c.GetAnalyticsSummary(ctx, repository.AllFields())
	if err != nil {
		t.Fatalf("GetAnalyticsSummary failed after recovery: %v", err)
	}
	if s.TotalUsers != 7 {
		t.Fatalf("TotalUsers = %d, want 7", s.TotalUsers)
	}
	if got := inner.summaryCalls.Load(); got != 2 {
		t.Fatalf("inner GetAnalyticsSummary ran %d times, want 2 (失敗はキャッシュしない)", got)
	}
}

// ■ 期限切れキーが解放されること
//
// 時系列とコミュニティのキャッシュはキー付きなので、条件の組み合わせが増える
// たびにエントリが増える。期限切れを消していないと、管理画面で期間を動かした
// 回数ぶんメモリに残り続ける。掃除は「新しい結果を入れるついで」に走る。

// fakeClock は掃除の間隔と TTL をまたぐために時間を進める。
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newClockedCache(t *testing.T, inner repository.AnalyticsRepository, ttl time.Duration) (*CachedAnalyticsRepository, *fakeClock) {
	t.Helper()
	clk := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	c := NewCachedAnalyticsRepository(inner, ttl, ttl)
	c.now = clk.now
	return c, clk
}

func TestTimeSeriesCacheReleasesExpiredKeys(t *testing.T) {
	inner := &fakeAnalyticsRepo{}
	const ttl = time.Minute
	c, clk := newClockedCache(t, inner, ttl)
	ctx := context.Background()

	// 期間違いで3つのキーを作る。
	for _, from := range []string{"2026-01-01", "2026-01-02", "2026-01-03"} {
		if _, err := c.GetTimeSeries(ctx, "day", from, "2026-01-31", repository.AllFields()); err != nil {
			t.Fatalf("GetTimeSeries failed: %v", err)
		}
	}
	if got := len(c.tsEntries); got != 3 {
		t.Fatalf("cached time series keys = %d, want 3", got)
	}

	// TTL と掃除間隔の両方を越えてから、別の条件で1つ取る。
	clk.advance(ttl + cacheSweepInterval)
	if _, err := c.GetTimeSeries(ctx, "hour", "2026-02-01", "2026-02-02", repository.AllFields()); err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}

	if got := len(c.tsEntries); got != 1 {
		t.Fatalf("cached time series keys after the sweep = %d, want 1 (期限切れ3つを解放)", got)
	}
	if _, ok := c.tsEntries["hour:2026-02-01:2026-02-02:*"]; !ok {
		t.Fatalf("the sweep dropped the fresh entry; keys = %v", keysOf(c.tsEntries))
	}
}

func TestCommunityCacheReleasesExpiredKeys(t *testing.T) {
	inner := &fakeAnalyticsRepo{}
	const ttl = time.Minute
	c, clk := newClockedCache(t, inner, ttl)
	ctx := context.Background()

	for _, off := range []int{0, 20, 40} {
		if _, _, err := c.GetCommunityAnalytics(ctx, repository.PageQuery{Limit: 20, Offset: off, WithTotal: true}); err != nil {
			t.Fatalf("GetCommunityAnalytics failed: %v", err)
		}
	}
	if got := len(c.commEntries); got != 3 {
		t.Fatalf("cached community keys = %d, want 3", got)
	}

	clk.advance(ttl + cacheSweepInterval)
	if _, _, err := c.GetCommunityAnalytics(ctx, repository.PageQuery{Limit: 50, Offset: 0, WithTotal: false}); err != nil {
		t.Fatalf("GetCommunityAnalytics failed: %v", err)
	}

	if got := len(c.commEntries); got != 1 {
		t.Fatalf("cached community keys after the sweep = %d, want 1 (期限切れ3つを解放)", got)
	}
	if _, ok := c.commEntries["50:0:false"]; !ok {
		t.Fatalf("the sweep dropped the fresh entry; keys = %v", keysOf(c.commEntries))
	}
}

// 掃除は最短間隔に1回だけ。まだ間隔が来ていないなら、期限切れでもなめない
// （毎回全キーを走査しないための約束）。
func TestCacheSweepRespectsMinimumInterval(t *testing.T) {
	inner := &fakeAnalyticsRepo{}
	const ttl = time.Second
	c, clk := newClockedCache(t, inner, ttl)
	ctx := context.Background()

	if _, err := c.GetTimeSeries(ctx, "day", "2026-01-01", "2026-01-31", repository.AllFields()); err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	// 1件目の保存で lastSweep が動いているので、ここからは間隔待ち。
	clk.advance(ttl + time.Millisecond)
	if _, err := c.GetTimeSeries(ctx, "day", "2026-01-02", "2026-01-31", repository.AllFields()); err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	if got := len(c.tsEntries); got != 2 {
		t.Fatalf("cached time series keys = %d, want 2 (掃除間隔前なので残る)", got)
	}

	clk.advance(cacheSweepInterval)
	if _, err := c.GetTimeSeries(ctx, "day", "2026-01-03", "2026-01-31", repository.AllFields()); err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	if got := len(c.tsEntries); got != 1 {
		t.Fatalf("cached time series keys after the sweep = %d, want 1", got)
	}
}

// 期限切れのエントリは、掃除される前でも結果として返さないこと。
func TestExpiredEntryIsNotServed(t *testing.T) {
	inner := &fakeAnalyticsRepo{}
	const ttl = time.Minute
	c, clk := newClockedCache(t, inner, ttl)
	ctx := context.Background()

	if _, err := c.GetTimeSeries(ctx, "day", "2026-01-01", "2026-01-31", repository.AllFields()); err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	if _, err := c.GetTimeSeries(ctx, "day", "2026-01-01", "2026-01-31", repository.AllFields()); err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	if got := inner.tsCalls.Load(); got != 1 {
		t.Fatalf("inner GetTimeSeries ran %d times within the TTL, want 1", got)
	}

	clk.advance(ttl)
	if _, err := c.GetTimeSeries(ctx, "day", "2026-01-01", "2026-01-31", repository.AllFields()); err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	if got := inner.tsCalls.Load(); got != 2 {
		t.Fatalf("inner GetTimeSeries ran %d times after the TTL, want 2", got)
	}
}

func keysOf[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// ■ 「一部だけ計算した結果」を全部要求した側へ配らないこと
//
// 要求されたフィールドだけ集計するようになったので、キャッシュのキーに
// 「何を計算したか」を含めないと、PageQuery.WithTotal で起きたのと同型の
// 事故（数えなかった total=0 を、total を選んだ側へ返す）がフィールドの
// 数だけ起きる。
func TestSummaryCacheKeyIncludesRequestedFields(t *testing.T) {
	// 要求されたフィールドだけ埋める内側。実物と同じ振る舞いにしてある。
	inner := &fieldAwareAnalyticsRepo{}
	c := NewCachedAnalyticsRepository(inner, time.Minute, time.Minute)
	ctx := context.Background()

	fewer := repository.NewFieldSet([]string{"totalUsers"})
	more := repository.NewFieldSet([]string{"totalUsers", "totalPosts"})

	// 先に「totalUsers だけ」を計算させてキャッシュに載せる。
	if _, err := c.GetAnalyticsSummary(ctx, fewer); err != nil {
		t.Fatalf("GetAnalyticsSummary failed: %v", err)
	}
	// 続けて totalPosts も要求する。ここで前の結果を使い回すと TotalPosts が 0 になる。
	got, err := c.GetAnalyticsSummary(ctx, more)
	if err != nil {
		t.Fatalf("GetAnalyticsSummary failed: %v", err)
	}
	if got.TotalPosts != 42 {
		t.Fatalf("TotalPosts = %d, want 42（totalUsers だけ計算した結果を使い回している）", got.TotalPosts)
	}

	// 同じ組み合わせならキャッシュが効く。
	before := inner.calls.Load()
	if _, err := c.GetAnalyticsSummary(ctx, more); err != nil {
		t.Fatalf("GetAnalyticsSummary failed: %v", err)
	}
	if inner.calls.Load() != before {
		t.Fatalf("同じ組み合わせなのに内側が再実行された")
	}

	// 全部要求（ゼロ値）は、部分計算の結果とは別のキー。
	full, err := c.GetAnalyticsSummary(ctx, repository.AllFields())
	if err != nil {
		t.Fatalf("GetAnalyticsSummary failed: %v", err)
	}
	if full.TotalLikes != 7 {
		t.Fatalf("TotalLikes = %d, want 7（部分計算の結果を使い回している）", full.TotalLikes)
	}
}

// 時系列も同じ。系列の組み合わせがキーに入っていること。
func TestTimeSeriesCacheKeyIncludesRequestedSeries(t *testing.T) {
	inner := &fieldAwareAnalyticsRepo{}
	c := NewCachedAnalyticsRepository(inner, time.Minute, time.Minute)
	ctx := context.Background()

	if _, err := c.GetTimeSeries(ctx, "day", "2026-01-01", "2026-01-31", repository.NewFieldSet([]string{"posts"})); err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	points, err := c.GetTimeSeries(ctx, "day", "2026-01-01", "2026-01-31", repository.NewFieldSet([]string{"posts", "messages"}))
	if err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	if len(points) != 1 || points[0].Messages != 5 {
		t.Fatalf("points = %+v, want Messages=5（posts だけ引いた結果を使い回している）", points[0])
	}
}

// InvalidateSummary は組み合わせを問わず全部捨てること。
func TestInvalidateSummaryDropsEveryFieldSet(t *testing.T) {
	inner := &fieldAwareAnalyticsRepo{}
	c := NewCachedAnalyticsRepository(inner, time.Minute, time.Minute)
	ctx := context.Background()

	for _, f := range []repository.FieldSet{
		repository.NewFieldSet([]string{"totalUsers"}),
		repository.NewFieldSet([]string{"totalPosts"}),
		repository.AllFields(),
	} {
		if _, err := c.GetAnalyticsSummary(ctx, f); err != nil {
			t.Fatalf("GetAnalyticsSummary failed: %v", err)
		}
	}
	if got := len(c.summaryEntries); got != 3 {
		t.Fatalf("cached summary keys = %d, want 3", got)
	}

	c.InvalidateSummary()
	if got := len(c.summaryEntries); got != 0 {
		t.Fatalf("cached summary keys after InvalidateSummary = %d, want 0", got)
	}
}

// fieldAwareAnalyticsRepo は「要求されたフィールドだけ埋める」内側。
// 実物（infra/mysql）と同じ振る舞いをさせないと、キーの取り違えが表に出ない。
type fieldAwareAnalyticsRepo struct {
	calls atomic.Int64
}

func (f *fieldAwareAnalyticsRepo) GetAnalyticsSummary(ctx context.Context, fields repository.FieldSet) (*model.AnalyticsSummary, error) {
	f.calls.Add(1)
	s := &model.AnalyticsSummary{}
	if fields.Wants("totalUsers") {
		s.TotalUsers = 1
	}
	if fields.Wants("totalPosts") {
		s.TotalPosts = 42
	}
	if fields.Wants("totalLikes") {
		s.TotalLikes = 7
	}
	return s, nil
}

func (f *fieldAwareAnalyticsRepo) GetTimeSeries(ctx context.Context, granularity, from, to string, series repository.FieldSet) ([]*model.TimeSeriesPoint, error) {
	f.calls.Add(1)
	p := &model.TimeSeriesPoint{Label: from}
	if series.Wants("posts") {
		p.Posts = 3
	}
	if series.Wants("messages") {
		p.Messages = 5
	}
	return []*model.TimeSeriesPoint{p}, nil
}

func (f *fieldAwareAnalyticsRepo) GetCommunityAnalytics(ctx context.Context, q repository.PageQuery) ([]*model.CommunityStatItem, int, error) {
	f.calls.Add(1)
	return nil, 0, nil
}

var _ repository.AnalyticsRepository = (*fieldAwareAnalyticsRepo)(nil)
