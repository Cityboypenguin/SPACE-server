package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetRepliesByIDUseCase interface {
	Execute(ctx context.Context, postID int64) ([]*model.Post, error)
}

var _ GetRepliesByIDUseCase = &GetRepliesByIDInteractor{}

type GetRepliesByIDInteractor struct {
	postRepo repository.PostRepository
}

func NewGetRepliesByIDUseCase(postRepo repository.PostRepository) GetRepliesByIDUseCase {
	return &GetRepliesByIDInteractor{
		postRepo: postRepo,
	}
}

func (uc *GetRepliesByIDInteractor) Execute(ctx context.Context, postID int64) ([]*model.Post, error) {
	return uc.postRepo.GetRepliesByID(ctx, postID)
}
