package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type FavoriteRepository interface {
	GetFavoriteByID(ctx context.Context, id int64) (*model.Favorite, error)
	CreateFavorite(ctx context.Context, favorite *model.Favorite) (int64, error)
	DeleteFavorite(ctx context.Context, id int64) (bool, error)
	GetFavoritesByPostID(ctx context.Context, postID int64) ([]*model.Favorite, error)
	GetFavoritesByUserID(ctx context.Context, userID int64) ([]*model.Favorite, error)
	ListFavorites(ctx context.Context) ([]*model.Favorite, error)
}
