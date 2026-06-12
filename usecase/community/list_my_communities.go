package community

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListMyCommunitiesUseCase interface {
	Execute(ctx context.Context, limit, offset int) ([]*model.Community, int, error)
}

var _ ListMyCommunitiesUseCase = &ListMyCommunitiesInteractor{}

type ListMyCommunitiesInteractor struct {
	communityRepo repository.CommunityRepository
}

func NewListMyCommunitiesUseCase(communityRepo repository.CommunityRepository) ListMyCommunitiesUseCase {
	return &ListMyCommunitiesInteractor{communityRepo: communityRepo}
}

func (uc *ListMyCommunitiesInteractor) Execute(ctx context.Context, limit, offset int) ([]*model.Community, int, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, 0, err
	}
	return uc.communityRepo.ListCommunitiesByUserID(ctx, claims.ID, limit, offset)
}
