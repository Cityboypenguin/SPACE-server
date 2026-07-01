package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type AnalyticsRepository interface {
	GetAnalyticsSummary(ctx context.Context) (*model.AnalyticsSummary, error)
	GetCommunityAnalytics(ctx context.Context, limit, offset int) ([]*model.CommunityStatItem, int, error)
	GetTimeSeries(ctx context.Context, granularity, from, to string) ([]*model.TimeSeriesPoint, error)
}
