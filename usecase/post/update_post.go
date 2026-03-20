package post

import (
	"context"
	"errors"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdatePostUseCase interface {
	Execute(ctx context.Context, param model.UpdatePostParam) (*model.Post, error)
}

var _ UpdatePostUseCase = &UpdatePostInteractor{}

type UpdatePostInteractor struct {
	PostRepository repository.PostRepository
}

func (i *UpdatePostInteractor) Execute(ctx context.Context, param model.UpdatePostParam) (*model.Post, error) {
	p, err := i.PostRepository.GetPost(ctx, param.ID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.New("post not found")
	}

	p.Content = param.Content
	p.UpdatedAt = time.Now()

	if err := i.PostRepository.SavePost(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}
