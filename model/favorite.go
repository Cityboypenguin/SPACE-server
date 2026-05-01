package model

import "time"

type Favorite struct {
	ID        int64
	User      *User
	Post      *Post
	CreatedAt time.Time
}

type CreateFavoriteParam struct {
	UserID int64
	PostID int64
}

func (f *Favorite) CreateFavorite(param CreateFavoriteParam) {
	f.User = &User{ID: param.UserID}
	f.Post = &Post{ID: param.PostID}
	f.CreatedAt = time.Now()
}
