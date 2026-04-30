package comment

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreateCommentUseCase interface {
	Execute(ctx context.Context, param model.CreateCommentParam) (*model.Comment, error)
}

var _ CreateCommentUseCase = &CreateCommentInteractor{}

type CreateCommentInteractor struct {
	commentRepo repository.CommentRepository
}

func NewCreateCommentUseCase(commentRepo repository.CommentRepository) CreateCommentUseCase {
	return &CreateCommentInteractor{
		commentRepo: commentRepo,
	}
}

func (uc *CreateCommentInteractor) Execute(ctx context.Context, param model.CreateCommentParam) (*model.Comment, error) {
	comment := &model.Comment{}
	comment.CreateComment(param)

	err := uc.commentRepo.SaveComment(ctx, comment)
	if err != nil {
		return nil, err
	}

	return comment, nil
}
