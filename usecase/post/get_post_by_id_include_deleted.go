package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetPostByIDIncludeDeletedUseCase interface {
	Execute(ctx context.Context, id int64) (*model.Post, error)
}

var _ GetPostByIDIncludeDeletedUseCase = &GetPostByIDIncludeDeletedInteractor{}

type GetPostByIDIncludeDeletedInteractor struct {
	postRepo repository.PostRepository
}

func NewGetPostByIDIncludeDeletedUseCase(postRepo repository.PostRepository) GetPostByIDIncludeDeletedUseCase {
	return &GetPostByIDIncludeDeletedInteractor{
		postRepo: postRepo,
	}
}

func (uc *GetPostByIDIncludeDeletedInteractor) Execute(ctx context.Context, id int64) (*model.Post, error) {
	return uc.postRepo.GetPostByIDIncludeDeleted(ctx, id)
}
