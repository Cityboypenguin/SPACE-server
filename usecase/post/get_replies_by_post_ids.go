package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetRepliesByPostIDsUseCase interface {
	Execute(ctx context.Context, parentIDs []int64) (map[int64][]*model.Post, error)
}

var _ GetRepliesByPostIDsUseCase = &GetRepliesByPostIDsInteractor{}

type GetRepliesByPostIDsInteractor struct {
	postRepo repository.PostRepository
}

func NewGetRepliesByPostIDsUseCase(postRepo repository.PostRepository) GetRepliesByPostIDsUseCase {
	return &GetRepliesByPostIDsInteractor{
		postRepo: postRepo,
	}
}

func (uc *GetRepliesByPostIDsInteractor) Execute(ctx context.Context, parentIDs []int64) (map[int64][]*model.Post, error) {
	return uc.postRepo.GetRepliesByPostIDs(ctx, parentIDs)
}
