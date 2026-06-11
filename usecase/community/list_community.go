package community

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListAllCommunitiesUseCase interface {
	Execute(ctx context.Context, limit, offset int) ([]*model.Community, int, error)
}

var _ ListAllCommunitiesUseCase = &ListAllCommunitiesInteractor{}

type ListAllCommunitiesInteractor struct {
	communityRepo repository.CommunityRepository
}

func NewListAllCommunitiesUseCase(communityRepo repository.CommunityRepository) ListAllCommunitiesUseCase {
	return &ListAllCommunitiesInteractor{communityRepo: communityRepo}
}

func (uc *ListAllCommunitiesInteractor) Execute(ctx context.Context, limit, offset int) ([]*model.Community, int, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}
	return uc.communityRepo.ListAllCommunities(ctx, limit, offset)
}
