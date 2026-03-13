package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type PostRepository interface {
	GetPost(ctx context.Context, id int64) (*model.Post, error)
	GetPosts(ctx context.Context) ([]*model.Post, error)
	SavePost(ctx context.Context, post *model.Post) error
}
