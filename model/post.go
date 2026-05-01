package model

import "time"

type Post struct {
	ID        int64
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
	User      *User
	Favorites []*Favorite
	Parent    *Post
	Replies   []*Post
}

type CreatePostParam struct {
	UserID    int64
	Content   string
	ParentID  *int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UpdatePostParam struct {
	Content *string
}

func (p *Post) CreatePost(param CreatePostParam) {
	p.User = &User{ID: param.UserID}
	p.Content = param.Content
	p.CreatedAt = param.CreatedAt
	p.UpdatedAt = param.UpdatedAt
	if param.ParentID != nil {
		p.Parent = &Post{ID: *param.ParentID}
	}
}

func (p *Post) UpdatePost(param UpdatePostParam) {
	if param.Content != nil {
		p.Content = *param.Content
	}
	p.UpdatedAt = time.Now()
}
