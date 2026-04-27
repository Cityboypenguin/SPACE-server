package community

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdateCommunityUseCase interface {
	Execute(ctx context.Context, c *model.Community) error
}

var _ UpdateCommunityUseCase = &UpdateCommunityInteractor{}

type UpdateCommunityInteractor struct {
	communityRepo repository.CommunityRepository
}

func NewUpdateCommunityUseCase(communityRepo repository.CommunityRepository) UpdateCommunityUseCase {
	return &UpdateCommunityInteractor{
		communityRepo: communityRepo,
	}
}

func (uc *UpdateCommunityInteractor) Execute(ctx context.Context, c *model.Community) error {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return err
	}

	if err := uc.communityRepo.UpdateCommunity(ctx, c); err != nil {
		return err
	}
	return nil
}
