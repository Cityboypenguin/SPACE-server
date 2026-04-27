package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type CommunityRepository interface {
	SaveCommunity(ctx context.Context, c *model.Community) error
	GetCommunityByID(ctx context.Context, id int64) (*model.Community, error)
	SearchCommunitiesByName(ctx context.Context, name string) ([]*model.Community, error)
	UpdateCommunity(ctx context.Context, c *model.Community) error
	DeleteCommunity(ctx context.Context, id int64) (bool, error)
	ListCommunities(ctx context.Context) ([]*model.Community, error)
}
