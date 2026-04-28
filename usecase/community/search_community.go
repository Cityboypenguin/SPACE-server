package community

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SearchCommunityUseCase interface {
	Execute(ctx context.Context, name string) ([]*model.Community, error)
}

var _ SearchCommunityUseCase = &SearchCommunityInteractor{}

type SearchCommunityInteractor struct {
	communityRepo repository.CommunityRepository
}

func NewSearchCommunityUseCase(communityRepo repository.CommunityRepository) SearchCommunityUseCase {
	return &SearchCommunityInteractor{communityRepo: communityRepo}
}

func (uc *SearchCommunityInteractor) Execute(ctx context.Context, name string) ([]*model.Community, error) {
	_, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	return uc.communityRepo.SearchCommunities(ctx, name)
}
