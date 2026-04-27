package community

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListCommunityUseCase interface {
	Execute(ctx context.Context) ([]*model.Community, error)
}

var _ ListCommunityUseCase = &ListCommunityInteractor{}

type ListCommunityInteractor struct {
	communityRepo repository.CommunityRepository
}

func NewListCommunityUseCase(communityRepo repository.CommunityRepository) ListCommunityUseCase {
	return &ListCommunityInteractor{
		communityRepo: communityRepo,
	}
}

func (uc *ListCommunityInteractor) Execute(ctx context.Context) ([]*model.Community, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, err
	}

	cs, err := uc.communityRepo.ListCommunities(ctx)
	if err != nil {
		return nil, err
	}
	return cs, nil
}
