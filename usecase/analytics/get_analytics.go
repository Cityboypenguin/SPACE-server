package analytics

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// fields / series（repository.FieldSet）は「応答に出るフィールドはどれか」。PageQuery と同じく
// GraphQL の選択集合を見られるリゾルバで組み立て、ここはそのまま通すだけ。
// どの名前がどの集計を要するかはリポジトリ側の対応表が持つ（判定がここにも
// 現れると、フィールドを足したときに直す場所が増える）。
type GetAnalyticsUseCase interface {
	Execute(ctx context.Context, fields repository.FieldSet) (*model.AnalyticsSummary, error)
}

var _ GetAnalyticsUseCase = &GetAnalyticsInteractor{}

type GetAnalyticsInteractor struct {
	analyticsRepo repository.AnalyticsRepository
}

func NewGetAnalyticsUseCase(analyticsRepo repository.AnalyticsRepository) GetAnalyticsUseCase {
	return &GetAnalyticsInteractor{analyticsRepo: analyticsRepo}
}

func (uc *GetAnalyticsInteractor) Execute(ctx context.Context, fields repository.FieldSet) (*model.AnalyticsSummary, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	return uc.analyticsRepo.GetAnalyticsSummary(ctx, fields)
}

type GetCommunityAnalyticsUseCase interface {
	Execute(ctx context.Context, q repository.PageQuery) ([]*model.CommunityStatItem, int, error)
}

var _ GetCommunityAnalyticsUseCase = &GetCommunityAnalyticsInteractor{}

type GetCommunityAnalyticsInteractor struct {
	analyticsRepo repository.AnalyticsRepository
}

func NewGetCommunityAnalyticsUseCase(analyticsRepo repository.AnalyticsRepository) GetCommunityAnalyticsUseCase {
	return &GetCommunityAnalyticsInteractor{analyticsRepo: analyticsRepo}
}

func (uc *GetCommunityAnalyticsInteractor) Execute(ctx context.Context, q repository.PageQuery) ([]*model.CommunityStatItem, int, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}
	return uc.analyticsRepo.GetCommunityAnalytics(ctx, q)
}

type GetTimeSeriesUseCase interface {
	Execute(ctx context.Context, granularity, from, to string, series repository.FieldSet) ([]*model.TimeSeriesPoint, error)
}

var _ GetTimeSeriesUseCase = &GetTimeSeriesInteractor{}

type GetTimeSeriesInteractor struct {
	analyticsRepo repository.AnalyticsRepository
}

func NewGetTimeSeriesUseCase(analyticsRepo repository.AnalyticsRepository) GetTimeSeriesUseCase {
	return &GetTimeSeriesInteractor{analyticsRepo: analyticsRepo}
}

func (uc *GetTimeSeriesInteractor) Execute(ctx context.Context, granularity, from, to string, series repository.FieldSet) ([]*model.TimeSeriesPoint, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	return uc.analyticsRepo.GetTimeSeries(ctx, granularity, from, to, series)
}
