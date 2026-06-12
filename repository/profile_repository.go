package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type ProfileRepository interface {
	GetProfileByUserID(ctx context.Context, userID int64) (*model.Profile, error)
	SaveProfile(ctx context.Context, profile *model.Profile) error
	SetAvatarMedia(ctx context.Context, userID int64, mediaID int64) error
	ClearAvatarMedia(ctx context.Context, userID int64) error
}
