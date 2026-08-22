package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

type AnswerQuestionUseCase interface {
	Execute(ctx context.Context, questionID int64, body string) (*model.Answer, error)
}

var _ AnswerQuestionUseCase = &AnswerQuestionInteractor{}

type AnswerQuestionInteractor struct {
	questionRepo    repository.QuestionRepository
	answerRepo      repository.AnswerRepository
	requireWritable course.RequireWritableCourseRoomUseCase
}

func NewAnswerQuestionUseCase(questionRepo repository.QuestionRepository, answerRepo repository.AnswerRepository, requireWritable course.RequireWritableCourseRoomUseCase) AnswerQuestionUseCase {
	return &AnswerQuestionInteractor{questionRepo: questionRepo, answerRepo: answerRepo, requireWritable: requireWritable}
}

func (uc *AnswerQuestionInteractor) Execute(ctx context.Context, questionID int64, body string) (*model.Answer, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	q, err := uc.questionRepo.GetQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperr.NotFound("質問が見つかりません")
	}
	if _, err := uc.requireWritable.Execute(ctx, q.RoomID); err != nil {
		return nil, err
	}

	a := &model.Answer{
		QuestionID:   questionID,
		AuthorUserID: claims.ID,
		AuthorRole:   model.AuthorRoleStudent,
		Body:         body,
	}
	if err := uc.answerRepo.SaveAnswer(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}
