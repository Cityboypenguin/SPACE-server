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
	txManager   repository.TxManager
}

func NewSetAvatarUseCase(profileRepo repository.ProfileRepository, mediaRepo repository.MediaRepository, txManager repository.TxManager) SetAvatarUseCase {
	return &SetAvatarInteractor{profileRepo: profileRepo, mediaRepo: mediaRepo, txManager: txManager}
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
	var p *model.Profile
	err := uc.txManager.RunInTx(ctx, func(txCtx context.Context) error {
		if err := uc.mediaRepo.CreateMedia(txCtx, media); err != nil {
			return err
		}

		if err := uc.profileRepo.SetAvatarMedia(txCtx, userID, media.ID); err != nil {
			return err
		}

		var err error
		p, err = uc.profileRepo.GetProfileByUserID(txCtx, userID)
		return err
	})
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
