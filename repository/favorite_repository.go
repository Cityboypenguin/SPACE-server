package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type FavoriteRepository interface {
	GeteFavoriteByID(ctx context.Context, id string) (*model.Favorite, error)
	CreateFavorite(ctx context.Context, favorite *model.Favorite) (*model.Favorite, error)
	DeleteFavorite(ctx context.Context, id string) error
	GetFavoritesByPostID(ctx context.Context, postID string) ([]*model.Favorite, error)
}
