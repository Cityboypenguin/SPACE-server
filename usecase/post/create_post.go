package post

import (
	"context"

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

	err := uc.postRepo.SavePost(ctx, post)
	if err != nil {
		return nil, err
	}

	return post, nil
}
