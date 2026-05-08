package profile

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SetAvatarUseCase interface {
	Execute(ctx context.Context, userID int64, objectKey string) (*model.Profile, error)
}

type SetAvatarInteractor struct {
	profileRepo repository.ProfileRepository
}

func NewSetAvatarUseCase(profileRepo repository.ProfileRepository) SetAvatarUseCase {
	return &SetAvatarInteractor{profileRepo: profileRepo}
}

func (uc *SetAvatarInteractor) Execute(ctx context.Context, userID int64, objectKey string) (*model.Profile, error) {
	expectedPrefix := fmt.Sprintf("avatars/%d/", userID)
	if !strings.HasPrefix(objectKey, expectedPrefix) {
		return nil, fmt.Errorf("invalid object key")
	}

	if err := uc.profileRepo.SetAvatarKey(ctx, userID, objectKey); err != nil {
		return nil, err
	}

	p, err := uc.profileRepo.GetProfileByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return p, nil
}
