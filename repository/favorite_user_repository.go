package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type FavoriteUserRepository interface {
	CreateFavoriteUser(ctx context.Context, favoriteUser *model.FavoriteUser) (int64, error)
	DeleteFavoriteUser(ctx context.Context, userID int64, favoriteID int64) (bool, error)
	ListFavoriteUsers(ctx context.Context, userID int64, limit, offset int) ([]*model.FavoriteUser, int, error)
	ListFollowers(ctx context.Context, userID int64, limit, offset int) ([]*model.FavoriteUser, int, error)
	SearchFavoriteUsers(ctx context.Context, userID int64, keyword string) ([]*model.FavoriteUser, error)
	GetFavoriteUsersByUserID(ctx context.Context, userID int64) ([]*model.FavoriteUser, error)
}
