package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/domain"
	"github.com/Cityboypenguin/SPACE-server/graph/model"
)

type postRepo struct {
	postsMemory []*model.Post
}

func NewPostRepo() domain.PostRepository {
	return &postRepo{postsMemory: []*model.Post{}}
}

func (r *postRepo) Create(ctx context.Context, post *model.Post) (*model.Post, error) {
	r.postsMemory = append(r.postsMemory, post)
	return post, nil
}

func (r *postRepo) GetAll(ctx context.Context) ([]*model.Post, error) {
	return r.postsMemory, nil
}
