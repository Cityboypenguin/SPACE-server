package model

import "time"

type Favorite struct {
	ID        int64     `json:"ID"`
	UserID    string    `json:"user_id"`
	PostID    string    `json:"post_id"`
	CreatedAt time.Time `json:"createdAt"`
}

type CreateFavoriteParam struct {
	UserID    string    `json:"user_id"`
	PostID    string    `json:"post_id"`
	CreatedAt time.Time `json:"createdAt"`
}

func (f *Favorite) CreateFavorite(param CreateFavoriteParam) {
	f.UserID = param.UserID
	f.PostID = param.PostID
	f.CreatedAt = param.CreatedAt
}
