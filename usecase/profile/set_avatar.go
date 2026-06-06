package profile

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SetAvatarUseCase interface {
	Execute(ctx context.Context, userID int64, objectKey string) (*model.Profile, error)
}

type SetAvatarInteractor struct {
	profileRepo repository.ProfileRepository
	mediaRepo   repository.MediaRepository
}

func NewSetAvatarUseCase(profileRepo repository.ProfileRepository, mediaRepo repository.MediaRepository) SetAvatarUseCase {
	return &SetAvatarInteractor{profileRepo: profileRepo, mediaRepo: mediaRepo}
}

func (uc *SetAvatarInteractor) Execute(ctx context.Context, userID int64, objectKey string) (*model.Profile, error) {
	expectedPrefix := fmt.Sprintf("avatars/%d/", userID)
	if !strings.HasPrefix(objectKey, expectedPrefix) {
		return nil, fmt.Errorf("invalid object key")
	}

	media := &model.Media{
		UploaderUserID: userID,
		StorageKey:     objectKey,
		ContentType:    contentTypeFromKey(objectKey),
		CreatedAt:      time.Now(),
	}
	if err := uc.mediaRepo.CreateMedia(ctx, media); err != nil {
		return nil, err
	}

	if err := uc.profileRepo.SetAvatarMedia(ctx, userID, media.ID); err != nil {
		return nil, err
	}

	p, err := uc.profileRepo.GetProfileByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return p, nil
}

func contentTypeFromKey(key string) string {
	switch {
	case strings.HasSuffix(key, ".jpg"):
		return "image/jpeg"
	case strings.HasSuffix(key, ".png"):
		return "image/png"
	case strings.HasSuffix(key, ".webp"):
		return "image/webp"
	case strings.HasSuffix(key, ".gif"):
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}
