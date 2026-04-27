package community

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetCommunityUseCase interface {
	Execute(ctx context.Context, id int64) (*model.Community, error)
}

var _ GetCommunityUseCase = &GetCommunityInteractor{}

type GetCommunityInteractor struct {
	communityRepo repository.CommunityRepository
}

func NewGetCommunityUseCase(communityRepo repository.CommunityRepository) GetCommunityUseCase {
	return &GetCommunityInteractor{
		communityRepo: communityRepo,
	}
}

func (uc *GetCommunityInteractor) Execute(ctx context.Context, id int64) (*model.Community, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, err
	}

	c, err := uc.communityRepo.GetCommunityByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return c, nil
}
