package model

import "time"

type Comment struct {
	ID        int64     `json:"ID"`
	User      *User     `json:"user"`
	Post      *Post     `json:"post"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CreateCommentParam struct {
	UserID    int64     `json:"user_id"`
	PostID    int64     `json:"post_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type UpdateCommentParam struct {
	Content *string `json:"content,omitempty"`
}

func (c *Comment) CreateComment(param CreateCommentParam) {
	c.User = &User{ID: param.UserID}
	c.Post = &Post{ID: param.PostID}
	c.Content = param.Content
	c.CreatedAt = param.CreatedAt
	c.UpdatedAt = param.UpdatedAt
}

func (c *Comment) UpdateComment(param UpdateCommentParam) {
	if param.Content != nil {
		c.Content = *param.Content
	}
	c.UpdatedAt = time.Now()
}
