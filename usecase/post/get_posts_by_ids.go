package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetPostsByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) ([]*model.Post, error)
}

var _ GetPostsByIDsUseCase = &GetPostsByIDsInteractor{}

type GetPostsByIDsInteractor struct {
	postRepo repository.PostRepository
}

func NewGetPostsByIDsUseCase(postRepo repository.PostRepository) GetPostsByIDsUseCase {
	return &GetPostsByIDsInteractor{postRepo: postRepo}
}

func (uc *GetPostsByIDsInteractor) Execute(ctx context.Context, ids []int64) ([]*model.Post, error) {
	return uc.postRepo.GetPostsByIDs(ctx, ids)
}
