package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetPostByIDUseCase interface {
	Execute(ctx context.Context, id int64) (*model.Post, error)
}

var _ GetPostByIDUseCase = &GetPostByIDInteractor{}

type GetPostByIDInteractor struct {
	postRepo repository.PostRepository
}

func NewGetPostByIDUseCase(postRepo repository.PostRepository) GetPostByIDUseCase {
	return &GetPostByIDInteractor{
		postRepo: postRepo,
	}
}

func (uc *GetPostByIDInteractor) Execute(ctx context.Context, id int64) (*model.Post, error) {
	return uc.postRepo.GetPostByID(ctx, id)
}
