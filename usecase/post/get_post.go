package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetPostUseCase interface {
	Execute(context.Context, int64) (*model.Post, error)
}

var _ GetPostUseCase = &GetPostInteractor{}

type GetPostInteractor struct {
	PostRepository repository.PostRepository
}

func (i *GetPostInteractor) Execute(ctx context.Context, id int64) (*model.Post, error) {
	return i.PostRepository.GetPost(ctx, id)
}
