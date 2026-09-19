package community

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreateCommunityUseCase interface {
	Execute(ctx context.Context, name, description string, avatarKey *string) (*model.Community, error)
}

var _ CreateCommunityUseCase = &CreateCommunityInteractor{}

type CreateCommunityInteractor struct {
	communityRepo repository.CommunityRepository
}

func NewCreateCommunityUseCase(communityRepo repository.CommunityRepository) CreateCommunityUseCase {
	return &CreateCommunityInteractor{communityRepo: communityRepo}
}

func (uc *CreateCommunityInteractor) Execute(ctx context.Context, name, description string, avatarKey *string) (*model.Community, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	var avatar *repository.UpdateCommunityAvatarParam
	if avatarKey != nil && *avatarKey != "" {
		if err := validateCommunityAvatarKey(claims.ID, *avatarKey); err != nil {
			return nil, err
		}
		avatar = &repository.UpdateCommunityAvatarParam{
			UploaderUserID: claims.ID,
			StorageKey:     *avatarKey,
			ContentType:    contentTypeFromKey(*avatarKey),
		}
	}

	return uc.communityRepo.SaveCommunityWithRoom(ctx, name, description, avatar, claims.ID)
}

func validateCommunityAvatarKey(userID int64, key string) error {
	prefix := fmt.Sprintf("community-icons/%d/", userID)
	if !strings.HasPrefix(key, prefix) {
		return fmt.Errorf("invalid community avatar key")
	}
	return nil
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
