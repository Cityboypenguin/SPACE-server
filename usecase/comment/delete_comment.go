package comment

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteCommentUseCase interface {
	Execute(ctx context.Context, id int64) (bool, error)
}

var _ DeleteCommentUseCase = &DeleteCommentInteractor{}

type DeleteCommentInteractor struct {
	commentRepo repository.CommentRepository
}

func NewDeleteCommentUseCase(commentRepo repository.CommentRepository) DeleteCommentUseCase {
	return &DeleteCommentInteractor{
		commentRepo: commentRepo,
	}
}

func (uc *DeleteCommentInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	return uc.commentRepo.DeleteComment(ctx, id)
}
