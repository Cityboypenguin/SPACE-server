package community

import (
	"context"
	"strings"
	"time"

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
	mediaRepo     repository.MediaRepository
}

func NewCreateCommunityUseCase(communityRepo repository.CommunityRepository, mediaRepo repository.MediaRepository) CreateCommunityUseCase {
	return &CreateCommunityInteractor{communityRepo: communityRepo, mediaRepo: mediaRepo}
}

func (uc *CreateCommunityInteractor) Execute(ctx context.Context, name, description string, avatarKey *string) (*model.Community, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	var avatarMediaID *int64
	if avatarKey != nil && *avatarKey != "" {
		media := &model.Media{
			UploaderUserID: claims.ID,
			StorageKey:     *avatarKey,
			ContentType:    contentTypeFromKey(*avatarKey),
			CreatedAt:      time.Now(),
		}
		if err := uc.mediaRepo.CreateMedia(ctx, media); err != nil {
			return nil, err
		}
		avatarMediaID = &media.ID
	}

	return uc.communityRepo.SaveCommunityWithRoom(ctx, name, description, avatarMediaID, claims.ID)
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
