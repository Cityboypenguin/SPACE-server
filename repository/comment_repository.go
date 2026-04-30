package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type CommentRepository interface {
	SaveComment(ctx context.Context, comment *model.Comment) error
	DeleteComment(ctx context.Context, id int64) (bool, error)
	GetCommentByID(ctx context.Context, id int64) (*model.Comment, error)
	GetCommentsByPostID(ctx context.Context, postID int64) ([]*model.Comment, error)
	UpdateComment(ctx context.Context, comment *model.Comment) error
	ListComments(ctx context.Context) ([]*model.Comment, error)
}
