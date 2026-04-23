package model

import "time"

type Post struct {
	ID        int64       `json:"ID"`
	Content   string      `json:"content"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
	User      *User       `json:"user"`
	Favorites []*Favorite `json:"favorites"`
	Parent    *Post       `json:"parent,omitempty"`
	Replies   []*Post     `json:"replies,omitempty"`
}

type CreatePostParam struct {
	UserID    int64     `json:"user_id"`
	Content   string    `json:"content"`
	ParentID  *int64    `json:"parent_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpdatePostParam struct {
	Content *string `json:"content,omitempty"`
}

func (p *Post) CreatePost(param CreatePostParam) {
	p.User = &User{ID: param.UserID}
	p.Content = param.Content
	p.CreatedAt = param.CreatedAt
	p.UpdatedAt = param.UpdatedAt
}

func (p *Post) UpdatePost(param UpdatePostParam) {
	if param.Content != nil {
		p.Content = *param.Content
	}
	p.UpdatedAt = time.Now()
}
