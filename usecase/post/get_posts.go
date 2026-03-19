package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetPostsUseCase interface {
	Execute(context.Context) ([]*model.Post, error)
}

var _ GetPostsUseCase = &GetPostsInteractor{}

type GetPostsInteractor struct {
	PostRepository repository.PostRepository
}

func (i *GetPostsInteractor) Execute(ctx context.Context) ([]*model.Post, error) {
	return i.PostRepository.GetAllPosts(ctx)
}
