package model

import "time"

type Favorite struct {
	ID        int64
	UserID    int64
	PostID    int64
	CreatedAt time.Time
}

type CreateFavoriteParam struct {
	UserID int64
	PostID int64
}

func CreateFavorite(param CreateFavoriteParam) *Favorite {
	return &Favorite{
		UserID:    param.UserID,
		PostID:    param.PostID,
		CreatedAt: time.Now(),
	}
}
