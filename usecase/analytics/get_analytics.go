package analytics

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetAnalyticsUseCase interface {
	Execute(ctx context.Context) (*model.AnalyticsSummary, error)
}

var _ GetAnalyticsUseCase = &GetAnalyticsInteractor{}

type GetAnalyticsInteractor struct {
	analyticsRepo repository.AnalyticsRepository
}

func NewGetAnalyticsUseCase(analyticsRepo repository.AnalyticsRepository) GetAnalyticsUseCase {
	return &GetAnalyticsInteractor{analyticsRepo: analyticsRepo}
}

func (uc *GetAnalyticsInteractor) Execute(ctx context.Context) (*model.AnalyticsSummary, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	return uc.analyticsRepo.GetAnalyticsSummary(ctx)
}

type GetCommunityAnalyticsUseCase interface {
	Execute(ctx context.Context, limit, offset int) ([]*model.CommunityStatItem, int, error)
}

var _ GetCommunityAnalyticsUseCase = &GetCommunityAnalyticsInteractor{}

type GetCommunityAnalyticsInteractor struct {
	analyticsRepo repository.AnalyticsRepository
}

func NewGetCommunityAnalyticsUseCase(analyticsRepo repository.AnalyticsRepository) GetCommunityAnalyticsUseCase {
	return &GetCommunityAnalyticsInteractor{analyticsRepo: analyticsRepo}
}

func (uc *GetCommunityAnalyticsInteractor) Execute(ctx context.Context, limit, offset int) ([]*model.CommunityStatItem, int, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}
	return uc.analyticsRepo.GetCommunityAnalytics(ctx, limit, offset)
}

type GetTimeSeriesUseCase interface {
	Execute(ctx context.Context, granularity, from, to string) ([]*model.TimeSeriesPoint, error)
}

var _ GetTimeSeriesUseCase = &GetTimeSeriesInteractor{}

type GetTimeSeriesInteractor struct {
	analyticsRepo repository.AnalyticsRepository
}

func NewGetTimeSeriesUseCase(analyticsRepo repository.AnalyticsRepository) GetTimeSeriesUseCase {
	return &GetTimeSeriesInteractor{analyticsRepo: analyticsRepo}
}

func (uc *GetTimeSeriesInteractor) Execute(ctx context.Context, granularity, from, to string) ([]*model.TimeSeriesPoint, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	return uc.analyticsRepo.GetTimeSeries(ctx, granularity, from, to)
}
