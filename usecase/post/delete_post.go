package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeletePostUseCase interface {
	Execute(ctx context.Context, id int64) (bool, error)
}

var _ DeletePostUseCase = &DeletePostInteractor{}

type DeletePostInteractor struct {
	postRepo repository.PostRepository
}

func NewDeletePostUseCase(postRepo repository.PostRepository) DeletePostUseCase {
	return &DeletePostInteractor{
		postRepo: postRepo,
	}
}

func (uc *DeletePostInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	return uc.postRepo.DeletePost(ctx, id)
}
