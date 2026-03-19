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

func (p *Post) Create(params CreatePostParam) error {
	p.AuthorID = params.AuthorID
	p.Content = params.Content
	p.CreatedAt = params.CreatedAt
	p.UpdatedAt = params.UpdatedAt

	return nil
}
