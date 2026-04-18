package model

import "time"

type Comment struct {
	ID        int64     `json:"ID"`
	UserID    string    `json:"user_id"`
	PostID    string    `json:"post_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CreateCommentParam struct {
	UserID    string    `json:"user_id"`
	PostID    string    `json:"post_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type UpdateCommentParam struct {
	Content *string `json:"content,omitempty"`
}

func (c *Comment) CreateComment(param CreateCommentParam) {
	c.UserID = param.UserID
	c.PostID = param.PostID
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
