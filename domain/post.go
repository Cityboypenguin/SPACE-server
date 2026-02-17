package domain

import (
	"context"
	"github.com/Cityboypenguin/SPACE-server/graph/model"
)

type PostRepository interface {
	Create(ctx context.Context, post *model.Post) (*model.Post, error)
	GetAll(ctx context.Context) ([]*model.Post, error)
}

type PostUsecase interface {
	CreatePost(ctx context.Context, content string, userId string) (*model.Post, error)
	GetPosts(ctx context.Context) ([]*model.Post, error)
}