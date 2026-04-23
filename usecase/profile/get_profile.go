package profile

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetProfileUseCase interface {
	Execute(ctx context.Context, userID int64) (*model.Profile, error)
}

type GetProfileInteractor struct {
	profileRepo repository.ProfileRepository
}

func NewGetProfileUseCase(profileRepo repository.ProfileRepository) GetProfileUseCase {
	return &GetProfileInteractor{
		profileRepo: profileRepo,
	}
}

func (uc *GetProfileInteractor) Execute(ctx context.Context, userID int64) (*model.Profile, error) {
	if _, err := authz.RequireSelfOrAdmin(ctx, userID); err != nil {
		return nil, err
	}

	return uc.profileRepo.GetProfileByUserID(ctx, userID)
}
