package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type CommunityRepository interface {
	SaveCommunity(ctx context.Context, c *model.Community) error
	GetCommunityByID(ctx context.Context, id int64) (*model.Community, error)
}
