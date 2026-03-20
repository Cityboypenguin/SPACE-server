package repository

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
)

var ErrPostNotFound = errors.New("post not found")

type PostRepository interface {
	SavePost(ctx context.Context, post *model.Post) error
	GetPost(ctx context.Context, id int64) (*model.Post, error)
	GetPostsByAuthorID(ctx context.Context, authorID int64) ([]*model.Post, error)
	GetAllPosts(ctx context.Context) ([]*model.Post, error)
	UpdatePost(ctx context.Context, post *model.Post) error
}