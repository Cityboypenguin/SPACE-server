package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type FavoriteUserRepository interface {
	CreateFavoriteUser(ctx context.Context, favoriteUser *model.FavoriteUser) (int64, error)
	DeleteFavoriteUser(ctx context.Context, userID int64, favoriteID int64) (bool, error)
	ListFavoriteUsers(ctx context.Context, userID int64, q PageQuery) ([]*model.FavoriteUser, int, error)
	ListFollowers(ctx context.Context, userID int64, q PageQuery) ([]*model.FavoriteUser, int, error)
	// この2つは以前は窓を取らず全件返していた（ListFavoriteUsers / ListFollowers は
	// 最初から PageQuery を取っている）。total は返さない＝呼び出し元が
	// [User!]! を返すので数える先が無い。
	SearchFavoriteUsers(ctx context.Context, userID int64, keyword string, q PageQuery) ([]*model.FavoriteUser, error)
	GetFavoriteUsersByUserID(ctx context.Context, userID int64, q PageQuery) ([]*model.FavoriteUser, error)
}
