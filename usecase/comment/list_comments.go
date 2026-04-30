package comment

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListCommentsUseCase interface {
	Execute(ctx context.Context) ([]*model.Comment, error)
}

var _ ListCommentsUseCase = &ListCommentsInteractor{}

type ListCommentsInteractor struct {
	commentRepo repository.CommentRepository
}

func NewListCommentsUseCase(commentRepo repository.CommentRepository) ListCommentsUseCase {
	return &ListCommentsInteractor{
		commentRepo: commentRepo,
	}
}

func (uc *ListCommentsInteractor) Execute(ctx context.Context) ([]*model.Comment, error) {
	return uc.commentRepo.ListComments(ctx)
}
