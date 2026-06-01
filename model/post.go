package model

import "time"

type Post struct {
	ID         int64
	Content    string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeteledAt  *time.Time
	UserID     int64
	ParentID   *int64
	DeletedAt  *time.Time
	ReplyCount int64
}

type CreatePostParam struct {
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
	UserID    int64
	ParentID  *int64
	MediaKeys []string
}

type UpdatePostParam struct {
	Content *string
}

func CreatePost(param CreatePostParam) *Post {
	return &Post{
		UserID:    param.UserID,
		Content:   param.Content,
		ParentID:  param.ParentID,
		CreatedAt: param.CreatedAt,
		UpdatedAt: param.UpdatedAt,
	}
}

func (p *Post) UpdatePost(param UpdatePostParam) {
	if param.Content != nil {
		p.Content = *param.Content
	}
	p.UpdatedAt = time.Now()
}

func (p *Post) DeletePost() {
	now := time.Now()
	p.DeletedAt = &now
	p.UpdatedAt = now
}
