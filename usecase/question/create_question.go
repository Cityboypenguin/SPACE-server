package question

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

type CreateQuestionUseCase interface {
	Execute(ctx context.Context, roomID int64, body string) (*model.Question, error)
}

var _ CreateQuestionUseCase = &CreateQuestionInteractor{}

type CreateQuestionInteractor struct {
	questionRepo    repository.QuestionRepository
	requireWritable course.RequireWritableCourseRoomUseCase
}

func NewCreateQuestionUseCase(questionRepo repository.QuestionRepository, requireWritable course.RequireWritableCourseRoomUseCase) CreateQuestionUseCase {
	return &CreateQuestionInteractor{questionRepo: questionRepo, requireWritable: requireWritable}
}

func (uc *CreateQuestionInteractor) Execute(ctx context.Context, roomID int64, body string) (*model.Question, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := uc.requireWritable.Execute(ctx, roomID); err != nil {
		return nil, err
	}

	q := &model.Question{
		RoomID:      roomID,
		AskerUserID: claims.ID,
		AuthorRole:  model.AuthorRoleStudent,
		Body:        body,
	}
	if err := uc.questionRepo.SaveQuestion(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}
