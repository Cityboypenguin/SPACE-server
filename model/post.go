package model

import "time"

type Post struct {
	ID            int64       `json:"ID"`
	UserID        int64       `json:"userId"`
	Content       string      `json:"content"`
	Picture       *string     `json:"picture,omitempty"`
	Movie         *string     `json:"movie,omitempty"`
	Hyperlink     *string     `json:"hyperlink,omitempty"`
	FavoriteCount int64       `json:"favoriteCount"`
	CreatedAt     time.Time   `json:"createdAt"`
	UpdatedAt     time.Time   `json:"updatedAt"`
	Comments      []*Comment  `json:"comments,omitempty"`
	Favorites     []*Favorite `json:"favorites,omitempty"`
}

type CreatePostParam struct {
	UserID    int64   `json:"userId"`
	Content   string  `json:"content"`
	Picture   *string `json:"picture,omitempty"`
	Movie     *string `json:"movie,omitempty"`
	Hyperlink *string `json:"hyperlink,omitempty"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UpdatePostParam struct {
	Content   *string `json:"content,omitempty"`
	Picture   *string `json:"picture,omitempty"`
	Movie     *string `json:"movie,omitempty"`
	Hyperlink *string `json:"hyperlink,omitempty"`
}

func (p *Post) CreatePost(param CreatePostParam) {
	p.UserID = param.UserID
	p.Content = param.Content
	p.Picture = param.Picture
	p.Movie = param.Movie
	p.Hyperlink = param.Hyperlink
	p.CreatedAt = param.CreatedAt
	p.UpdatedAt = param.UpdatedAt
}

func (p *Post) UpdatePost(param UpdatePostParam) {
	if param.Content != nil {
		p.Content = *param.Content
	}
	if param.Picture != nil {
		p.Picture = param.Picture
	}
	if param.Movie != nil {
		p.Movie = param.Movie
	}
	if param.Hyperlink != nil {
		p.Hyperlink = param.Hyperlink
	}
	p.UpdatedAt = time.Now()
}
