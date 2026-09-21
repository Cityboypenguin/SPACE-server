package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetRepliesByIDUseCase interface {
	Execute(ctx context.Context, postID int64, q repository.PageQuery) ([]*model.Post, error)
}

var _ GetRepliesByIDUseCase = &GetRepliesByIDInteractor{}

type GetRepliesByIDInteractor struct {
	postRepo repository.PostReplyReader
}

func NewGetRepliesByIDUseCase(postRepo repository.PostReplyReader) GetRepliesByIDUseCase {
	return &GetRepliesByIDInteractor{
		postRepo: postRepo,
	}
}

func (uc *GetRepliesByIDInteractor) Execute(ctx context.Context, postID int64, q repository.PageQuery) ([]*model.Post, error) {
	return uc.postRepo.GetRepliesByID(ctx, postID, q)
}
