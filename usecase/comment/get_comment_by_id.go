package comment

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetCommentByIDUseCase interface {
	Execute(ctx context.Context, id int64) (*model.Comment, error)
}

var _ GetCommentByIDUseCase = &GetCommentByIDInteractor{}

type GetCommentByIDInteractor struct {
	commentRepo repository.CommentRepository
}

func NewGetCommentByIDUseCase(commentRepo repository.CommentRepository) GetCommentByIDUseCase {
	return &GetCommentByIDInteractor{
		commentRepo: commentRepo,
	}
}

func (uc *GetCommentByIDInteractor) Execute(ctx context.Context, id int64) (*model.Comment, error) {
	return uc.commentRepo.GetCommentByID(ctx, id)
}
