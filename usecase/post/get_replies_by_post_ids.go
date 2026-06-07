package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetRepliesByPostIDsUseCase struct {
	postRepo repository.PostRepository
}

func NewGetRepliesByPostIDsUseCase(postRepo repository.PostRepository) *GetRepliesByPostIDsUseCase {
	return &GetRepliesByPostIDsUseCase{postRepo: postRepo}
}

func (uc *GetRepliesByPostIDsUseCase) Execute(ctx context.Context, parentIDs []int64) (map[int64][]*model.Post, error) {
	return uc.postRepo.GetRepliesByPostIDs(ctx, parentIDs)
}
