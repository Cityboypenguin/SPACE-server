package comment

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdateCommentUseCase interface {
	Execute(ctx context.Context, id int64, param model.UpdateCommentParam) (*model.Comment, error)
}

var _ UpdateCommentUseCase = &UpdateCommentInteractor{}

type UpdateCommentInteractor struct {
	commentRepo repository.CommentRepository
}

func NewUpdateCommentUseCase(commentRepo repository.CommentRepository) UpdateCommentUseCase {
	return &UpdateCommentInteractor{
		commentRepo: commentRepo,
	}
}

func (uc *UpdateCommentInteractor) Execute(ctx context.Context, id int64, param model.UpdateCommentParam) (*model.Comment, error) {
	comment, err := uc.commentRepo.GetCommentByID(ctx, id)
	if err != nil {
		return nil, err
	}

	comment.UpdateComment(param)

	err = uc.commentRepo.UpdateComment(ctx, comment)
	if err != nil {
		return nil, err
	}

	return comment, nil
}
