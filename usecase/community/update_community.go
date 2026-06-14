package community

import (
	"context"

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
}

func NewUpdateCommunityUseCase(communityRepo repository.CommunityRepository) UpdateCommunityUseCase {
	return &UpdateCommunityInteractor{
		communityRepo: communityRepo,
	}
}

func (uc *UpdateCommunityInteractor) Execute(ctx context.Context, c *model.Community, avatarKey *string) error {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return err
	}

	var avatar *repository.UpdateCommunityAvatarParam
	if avatarKey != nil && *avatarKey != "" {
		avatar = &repository.UpdateCommunityAvatarParam{
			UploaderUserID: claims.ID,
			StorageKey:     *avatarKey,
			ContentType:    contentTypeFromKey(*avatarKey),
		}
	}

	return uc.communityRepo.UpdateCommunity(ctx, c, avatar)
}
