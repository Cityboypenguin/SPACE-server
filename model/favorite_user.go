package model

import "time"

type FavoriteUser struct {
	ID             int64
	UserID         int64
	FavoriteUserID int64
	CreatedAt      time.Time
}

type CreateFavoriteUserParam struct {
	UserID         int64
	FavoriteUserID int64
}

func CreateFavoriteUser(param CreateFavoriteUserParam) *FavoriteUser {
	return &FavoriteUser{
		UserID:         param.UserID,
		FavoriteUserID: param.FavoriteUserID,
		CreatedAt:      time.Now(),
	}
}
