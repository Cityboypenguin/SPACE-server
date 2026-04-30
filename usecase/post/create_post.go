package post

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreatePostUseCase interface {
	Execute(ctx context.Context, param model.CreatePostParam) (*model.Post, error)
}

var _ CreatePostUseCase = &CreatePostInteractor{}

type CreatePostInteractor struct {
	postRepo repository.PostRepository
}

func NewCreatePostUseCase(postRepo repository.PostRepository) CreatePostUseCase {
	return &CreatePostInteractor{
		postRepo: postRepo,
	}
}

func (uc *CreatePostInteractor) Execute(ctx context.Context, param model.CreatePostParam) (*model.Post, error) {
	post := &model.Post{}
	post.CreatePost(param)

	now := time.Now()
	post.CreatedAt = now
	post.UpdatedAt = now

	id, err := uc.postRepo.CreatePost(ctx, post)
	if err != nil {
		return nil, err
	}

	post.ID = id

	return post, nil
}
