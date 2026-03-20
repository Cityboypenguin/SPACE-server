package model

import "time"

type Post struct {
	ID        int64     `json:"id"`
	AuthorID  int64     `json:"author_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreatePostParam struct {
	AuthorID  int64
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UpdatePostParam struct {
	ID      int64
	Content string
}

func NewPost(param CreatePostParam) *Post {
	return &Post{
		AuthorID:  param.AuthorID,
		Content:   param.Content,
		CreatedAt: param.CreatedAt,
		UpdatedAt: param.UpdatedAt,
	}
}
