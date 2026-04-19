package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type FavoriteRepository interface {
	GetFavoriteByID(ctx context.Context, id int64) (*model.Favorite, error)
	CreateFavorite(ctx context.Context, favorite *model.Favorite) (*model.Favorite, error)
	DeleteFavorite(ctx context.Context, id int64) error
	GetFavoritesByPostID(ctx context.Context, postID int64) ([]*model.Favorite, error)
}
