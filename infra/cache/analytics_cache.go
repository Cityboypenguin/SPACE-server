package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/metrics"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// CachedAnalyticsRepository wraps AnalyticsRepository with in-memory TTL caching.
// GetAnalyticsSummary is cached for summaryTTL (expensive: 35+ DB queries).
// GetTimeSeries and GetCommunityAnalytics are cached for keyedTTL.
type CachedAnalyticsRepository struct {
	inner repository.AnalyticsRepository

	// GetAnalyticsSummary — no params, single slot
	summaryMu      sync.RWMutex
	summaryVal     *model.AnalyticsSummary
	summaryExpires time.Time
	summaryTTL     time.Duration

	// GetTimeSeries — keyed by "granularity:from:to"
	tsMu      sync.RWMutex
	tsEntries map[string]tsEntry
	tsTTL     time.Duration

	// GetCommunityAnalytics — keyed by "limit:offset"
	commMu      sync.RWMutex
	commEntries map[string]commEntry
	commTTL     time.Duration
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
func (c *CachedAnalyticsRepository) InvalidateSummary() {
	c.summaryMu.Lock()
	c.summaryVal = nil
	c.summaryMu.Unlock()
}

func NewCachedAnalyticsRepository(
	inner repository.AnalyticsRepository,
	summaryTTL, keyedTTL time.Duration,
) *CachedAnalyticsRepository {
	return &CachedAnalyticsRepository{
		inner:       inner,
		summaryTTL:  summaryTTL,
		tsEntries:   make(map[string]tsEntry),
		tsTTL:       keyedTTL,
		commEntries: make(map[string]commEntry),
		commTTL:     keyedTTL,
	}
}

func (c *CachedAnalyticsRepository) GetAnalyticsSummary(ctx context.Context) (*model.AnalyticsSummary, error) {
	c.summaryMu.RLock()
	if c.summaryVal != nil && time.Now().Before(c.summaryExpires) {
		// DB由来フィールドはキャッシュのまま、in-memoryメトリクスだけ毎回最新値で上書き
		copy := *c.summaryVal
		copy.WebSocketConnections = metrics.Global.WSConnections()
		copy.SSEConnections = metrics.Global.SSEConnections()
		copy.ErrorRate5xx = metrics.Global.ErrorRate()
		copy.P50ResponseTimeMs = metrics.Global.Percentile(50)
		copy.P95ResponseTimeMs = metrics.Global.Percentile(95)
		copy.P99ResponseTimeMs = metrics.Global.Percentile(99)
		c.summaryMu.RUnlock()
		return &copy, nil
	}
	c.summaryMu.RUnlock()

	val, err := c.inner.GetAnalyticsSummary(ctx)
	if err != nil {
		return nil, err
	}

	c.summaryMu.Lock()
	c.summaryVal = val
	c.summaryExpires = time.Now().Add(c.summaryTTL)
	c.summaryMu.Unlock()

	return val, nil
}

func (c *CachedAnalyticsRepository) GetTimeSeries(ctx context.Context, granularity, from, to string) ([]*model.TimeSeriesPoint, error) {
	key := fmt.Sprintf("%s:%s:%s", granularity, from, to)

	c.tsMu.RLock()
	if e, ok := c.tsEntries[key]; ok && time.Now().Before(e.expires) {
		v := e.val
		c.tsMu.RUnlock()
		return v, nil
	}
	c.tsMu.RUnlock()

	val, err := c.inner.GetTimeSeries(ctx, granularity, from, to)
	if err != nil {
		return nil, err
	}

	c.tsMu.Lock()
	c.tsEntries[key] = tsEntry{val: val, expires: time.Now().Add(c.tsTTL)}
	c.tsMu.Unlock()

	return val, nil
}

func (c *CachedAnalyticsRepository) GetCommunityAnalytics(ctx context.Context, limit, offset int) ([]*model.CommunityStatItem, int, error) {
	key := fmt.Sprintf("%d:%d", limit, offset)

	c.commMu.RLock()
	if e, ok := c.commEntries[key]; ok && time.Now().Before(e.expires) {
		v, t := e.val, e.total
		c.commMu.RUnlock()
		return v, t, nil
	}
	c.commMu.RUnlock()

	val, total, err := c.inner.GetCommunityAnalytics(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	c.commMu.Lock()
	c.commEntries[key] = commEntry{val: val, total: total, expires: time.Now().Add(c.commTTL)}
	c.commMu.Unlock()

	return val, total, nil
}
