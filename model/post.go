package model

import "time"

type Post struct {
	ID         int64
	Content    string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	UserID     int64
	ParentID   *int64
	DeletedAt  *time.Time
	ReplyCount int64
}

type CreatePostParam struct {
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
	// UserID は投稿者。呼び出し側が入れた値は使われない（CreatePostUseCase が
	// ctx の認証情報で上書きする）。他人名義の投稿を作れる隙間を残さないため。
	UserID    int64
	ParentID  *int64
	MediaKeys []string
	// Video は付ける動画（無ければ nil）。画像（MediaKeys）とは同時に付けられない。
	Video *VideoInput
}

type UpdatePostParam struct {
	PostID  int64
	UserID  int64
	Content *string
	// RemoveVideo は付いている動画を外す。編集で動画を後から足すことはできない
	// （保存期間の数え始めが投稿の時刻とずれるため）。
	RemoveVideo bool
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
