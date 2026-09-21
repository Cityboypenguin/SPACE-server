package post // パッケージ名は君の環境に合わせる

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetRepliesByPostIDsIncludeDeletedUseCase interface {
	Execute(ctx context.Context, parentIDs []int64, q repository.PageQuery) (map[int64][]*model.Post, error)
}

var _ GetRepliesByPostIDsIncludeDeletedUseCase = &GetRepliesByPostIDsIncludeDeletedInteractor{}

type GetRepliesByPostIDsIncludeDeletedInteractor struct {
	postRepo repository.PostReplyReader
}

func NewGetRepliesByPostIDsIncludeDeletedUseCase(postRepo repository.PostReplyReader) GetRepliesByPostIDsIncludeDeletedUseCase {
	return &GetRepliesByPostIDsIncludeDeletedInteractor{
		postRepo: postRepo,
	}
}

func (uc *GetRepliesByPostIDsIncludeDeletedInteractor) Execute(ctx context.Context, parentIDs []int64, q repository.PageQuery) (map[int64][]*model.Post, error) {
	return uc.postRepo.GetRepliesByPostIDsIncludeDeleted(ctx, parentIDs, q)
}
