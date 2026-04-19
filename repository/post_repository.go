package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type PostRepository interface {
	SavePost(ctx context.Context, post *model.Post) error
	DeletePost(ctx context.Context, id int64) (bool, error)
	GetPostByID(ctx context.Context, id int64) (*model.Post, error)
	GetPostsByUserID(ctx context.Context, userID string) ([]*model.Post, error)
	ListPosts(ctx context.Context) ([]*model.Post, error)
	UpdatePost(ctx context.Context, post *model.Post) error
}
