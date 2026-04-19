package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreatePostUseCase interface {
	Execute(ctx context.Context, post *model.Post) error
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

func (uc *CreatePostInteractor) Execute(ctx context.Context, post *model.Post) error {
	return uc.postRepo.SavePost(ctx, post)
}
