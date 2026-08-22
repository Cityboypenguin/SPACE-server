package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type AnswerRepository interface {
	SaveAnswer(ctx context.Context, a *model.Answer) error
	GetAnswerByID(ctx context.Context, id int64) (*model.Answer, error)
	ListAnswersByQuestionID(ctx context.Context, questionID int64) ([]*model.Answer, error)
}
