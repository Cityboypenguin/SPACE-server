package community

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreateCommunityUseCase interface {
	Execute(ctx context.Context, name, description string) (*model.Community, error)
}

var _ CreateCommunityUseCase = &CreateCommunityInteractor{}

type CreateCommunityInteractor struct {
	communityRepo repository.CommunityRepository
}

func NewCreateCommunityUseCase(communityRepo repository.CommunityRepository) CreateCommunityUseCase {
	return &CreateCommunityInteractor{communityRepo: communityRepo}
}

func (uc *CreateCommunityInteractor) Execute(ctx context.Context, name, description string) (*model.Community, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	return uc.communityRepo.SaveCommunityWithRoom(ctx, name, description, claims.ID)
}
