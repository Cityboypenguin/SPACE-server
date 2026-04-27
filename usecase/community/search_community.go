package community

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SerarchCommunityUseCase interface {
	Execute(ctx context.Context, name string) ([]*model.Community, error)
}

var _ SerarchCommunityUseCase = &SearchCommunityInteractor{}

type SearchCommunityInteractor struct {
	communityRepo repository.CommunityRepository
}

func NewSearchCommunityUseCase(communityRepo repository.CommunityRepository) SerarchCommunityUseCase {
	return &SearchCommunityInteractor{
		communityRepo: communityRepo,
	}
}

func (uc *SearchCommunityInteractor) Execute(ctx context.Context, name string) ([]*model.Community, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, err
	}

	cs, err := uc.communityRepo.SearchCommunitiesByName(ctx, name)
	if err != nil {
		return nil, err
	}
	return cs, nil
}
