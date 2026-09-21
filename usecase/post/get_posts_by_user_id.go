package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetPostsByUserIDUseCase interface {
	Execute(ctx context.Context, userID int64, q repository.PageQuery) ([]*model.Post, int, error)
}

var _ GetPostsByUserIDUseCase = &GetPostsByUserIDInteractor{}

type GetPostsByUserIDInteractor struct {
	postRepo repository.PostLister
}

func NewGetPostsByUserIDUseCase(postRepo repository.PostLister) GetPostsByUserIDUseCase {
	return &GetPostsByUserIDInteractor{
		postRepo: postRepo,
	}
}

func (uc *GetPostsByUserIDInteractor) Execute(ctx context.Context, userID int64, q repository.PageQuery) ([]*model.Post, int, error) {
	return uc.postRepo.GetPostsByUserID(ctx, userID, q)
}
