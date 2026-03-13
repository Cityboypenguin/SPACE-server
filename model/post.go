package model

import (
	"time"
)

type Post struct {
	ID        int64     `json:"id"`
	Content   string    `json:"content"`
	Author    User      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

type CreatePostParam struct {
	Content   string
	Author    User
	CreatedAt time.Time
}

func (p *Post) CreatePost(params CreatePostParam) error {
	p.Content = params.Content
	p.Author = params.Author
	p.CreatedAt = params.CreatedAt

	return nil
}
