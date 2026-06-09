package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetRootPostUseCase interface {
	Execute(ctx context.Context, id int64) (*model.Post, error)
}

var _ GetRootPostUseCase = &GetRootPostInteractor{}

type GetRootPostInteractor struct {
	postRepo repository.PostRepository
}

func NewGetRootPostUseCase(postRepo repository.PostRepository) GetRootPostUseCase {
	return &GetRootPostInteractor{
		postRepo: postRepo,
	}
}

func (uc *GetRootPostInteractor) Execute(ctx context.Context, id int64) (*model.Post, error) {
	return uc.postRepo.GetRootPost(ctx, id)
}
