package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type ProfileRepository interface {
	GetProfileByUserID(ctx context.Context, userID string) (*model.Profile, error)
	SaveProfile(ctx context.Context, profile *model.Profile) error
}
