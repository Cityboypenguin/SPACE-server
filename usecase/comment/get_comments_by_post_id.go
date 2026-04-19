package comment

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetCommentsByPostIDUseCase interface {
	Execute(ctx context.Context, postID int64) ([]*model.Comment, error)
}

var _ GetCommentsByPostIDUseCase = &GetCommentsByPostIDInteractor{}

type GetCommentsByPostIDInteractor struct {
	commentRepo repository.CommentRepository
}

func NewGetCommentsByPostIDUseCase(commentRepo repository.CommentRepository) GetCommentsByPostIDUseCase {
	return &GetCommentsByPostIDInteractor{
		commentRepo: commentRepo,
	}
}

func (uc *GetCommentsByPostIDInteractor) Execute(ctx context.Context, postID int64) ([]*model.Comment, error) {
	return uc.commentRepo.GetCommentsByPostID(ctx, postID)
}
