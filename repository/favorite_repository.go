package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type FavoriteRepository interface {
	CreateFavorite(ctx context.Context, favorite *model.Favorite) (int64, error)
	DeleteFavorite(ctx context.Context, id int64) (bool, error)
	DeleteFavoriteByUserIDAndPostID(ctx context.Context, user_id int64, post_id int64) (bool, error)
	GetFavoriteByID(ctx context.Context, id int64) (*model.Favorite, error)
	GetFavoriteByUserIDAndPostID(ctx context.Context, user_id int64, post_id int64) (*model.Favorite, error)
	GetFavoritesByPostID(ctx context.Context, post_id int64) ([]*model.Favorite, error)
	GetFavoritesByUserID(ctx context.Context, user_id int64) ([]*model.Favorite, error)
	ListFavorites(ctx context.Context) ([]*model.Favorite, error)
	GetFavoritesByPostIDs(ctx context.Context, postIDs []int64) (map[int64][]*model.Favorite, error)
}
