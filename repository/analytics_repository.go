package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type AnalyticsRepository interface {
	// GetAnalyticsSummary は fields で要求されたものだけを埋める。要求されて
	// いないフィールドはゼロ値のまま返る（呼び出し側は応答に出さないので
	// 問題にならない）。どの名前がどの集計を要するかは実装側が持つ。
	GetAnalyticsSummary(ctx context.Context, fields FieldSet) (*model.AnalyticsSummary, error)
	GetCommunityAnalytics(ctx context.Context, q PageQuery) ([]*model.CommunityStatItem, int, error)
	// GetTimeSeries の series は TimeSeriesPoint のどの系列が要求されたか。
	// label は SQL を伴わないので常に埋まる。
	GetTimeSeries(ctx context.Context, granularity, from, to string, series FieldSet) ([]*model.TimeSeriesPoint, error)
}
