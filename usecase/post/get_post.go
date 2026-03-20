package post

import (
	"context"
	"errors"

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
	p, err := i.PostRepository.GetPost(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.New("post not found")
	}
	return p, nil
}
