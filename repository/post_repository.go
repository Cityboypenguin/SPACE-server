package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type PostRepository interface {
	SavePost(ctx context.Context, post *model.Post) error
	GetPost(ctx context.Context, id int64) (*model.Post, error)
	GetPostsByAuthorID(ctx context.Context, authorID int64) ([]*model.Post, error)
}