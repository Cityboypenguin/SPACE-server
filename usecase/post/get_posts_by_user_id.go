package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetPostsByUserIDUseCase interface {
	Execute(ctx context.Context, userID int64) ([]*model.Post, error)
}

var _ GetPostsByUserIDUseCase = &GetPostsByUserIDInteractor{}

type GetPostsByUserIDInteractor struct {
	postRepo repository.PostRepository
}

func NewGetPostsByUserIDUseCase(postRepo repository.PostRepository) GetPostsByUserIDUseCase {
	return &GetPostsByUserIDInteractor{
		postRepo: postRepo,
	}
}

func (uc *GetPostsByUserIDInteractor) Execute(ctx context.Context, userID int64) ([]*model.Post, error) {
	return uc.postRepo.GetPostsByUserID(ctx, userID)
}
