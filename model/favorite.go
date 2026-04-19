package model

import "time"

type Favorite struct {
	User      *User     `json:"user"`
	Post      *Post     `json:"post"`
	CreatedAt time.Time `json:"createdAt"`
}

type CreateFavoriteParam struct {
	UserID    int64     `json:"user_id"`
	PostID    int64     `json:"post_id"`
	CreatedAt time.Time `json:"createdAt"`
}

func (f *Favorite) CreateFavorite(param CreateFavoriteParam) {
	f.User = &User{ID: param.UserID}
	f.Post = &Post{ID: param.PostID}
	f.CreatedAt = param.CreatedAt
}
