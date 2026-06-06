package community

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdateCommunityUseCase interface {
	Execute(ctx context.Context, c *model.Community, avatarKey *string) error
}

var _ UpdateCommunityUseCase = &UpdateCommunityInteractor{}

type UpdateCommunityInteractor struct {
	communityRepo repository.CommunityRepository
	mediaRepo     repository.MediaRepository
}

func NewUpdateCommunityUseCase(communityRepo repository.CommunityRepository, mediaRepo repository.MediaRepository) UpdateCommunityUseCase {
	return &UpdateCommunityInteractor{
		communityRepo: communityRepo,
		mediaRepo:     mediaRepo,
	}
}

func (uc *UpdateCommunityInteractor) Execute(ctx context.Context, c *model.Community, avatarKey *string) error {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return err
	}

	if avatarKey != nil && *avatarKey != "" {
		media := &model.Media{
			UploaderUserID: claims.ID,
			StorageKey:     *avatarKey,
			ContentType:    contentTypeFromKey(*avatarKey),
			CreatedAt:      time.Now(),
		}
		if err := uc.mediaRepo.CreateMedia(ctx, media); err != nil {
			return err
		}
		c.AvatarMedia = media
	}

	return uc.communityRepo.UpdateCommunity(ctx, c)
}
