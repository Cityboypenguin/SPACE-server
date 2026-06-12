package profile

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteAvatarUseCase interface {
	Execute(ctx context.Context, userID int64) (*model.Profile, error)
}

type DeleteAvatarInteractor struct {
	profileRepo repository.ProfileRepository
}

func NewDeleteAvatarUseCase(profileRepo repository.ProfileRepository) DeleteAvatarUseCase {
	return &DeleteAvatarInteractor{profileRepo: profileRepo}
}

func (uc *DeleteAvatarInteractor) Execute(ctx context.Context, userID int64) (*model.Profile, error) {
	if err := uc.profileRepo.ClearAvatarMedia(ctx, userID); err != nil {
		return nil, err
	}

	p, err := uc.profileRepo.GetProfileByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return p, nil
}
