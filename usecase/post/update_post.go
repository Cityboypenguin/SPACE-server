package post

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdatepostUseCase interface {
	Execute(context.Context, int64, string) (*model.Post, error)
}

var _ UpdatepostUseCase = &UpdatePostInteractor{}

type UpdatePostInteractor struct {
	PostRepository repository.PostRepository
}

func (i *UpdatePostInteractor) Execute(ctx context.Context, id int64, content string) (*model.Post, error) {
	post, err := i.PostRepository.GetPost(ctx, id)
	if err != nil {
		return nil, err
	}
	post.Content = content
	post.UpdatedAt = time.Now()
	err = i.PostRepository.UpdatePost(ctx, post)
	if err != nil {
		return nil, err
	}
	return post, nil
}