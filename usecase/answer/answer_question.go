package answer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

const MaxMediaCount = 4

type MediaInput struct {
	StorageKey  string
	ContentType string
}

type AnswerQuestionUseCase interface {
	Execute(ctx context.Context, questionID int64, body string, mediaInputs []MediaInput) (*model.Answer, error)
}

var _ AnswerQuestionUseCase = &AnswerQuestionInteractor{}

type AnswerQuestionInteractor struct {
	questionRepo    repository.QuestionRepository
	answerRepo      repository.AnswerRepository
	mediaRepo       repository.MediaRepository
	txManager       repository.TxManager
	requireWritable course.RequireWritableCourseRoomUseCase
}

func NewAnswerQuestionUseCase(
	questionRepo repository.QuestionRepository,
	answerRepo repository.AnswerRepository,
	mediaRepo repository.MediaRepository,
	txManager repository.TxManager,
	requireWritable course.RequireWritableCourseRoomUseCase,
) AnswerQuestionUseCase {
	return &AnswerQuestionInteractor{
		questionRepo:    questionRepo,
		answerRepo:      answerRepo,
		mediaRepo:       mediaRepo,
		txManager:       txManager,
		requireWritable: requireWritable,
	}
}

func (uc *AnswerQuestionInteractor) Execute(ctx context.Context, questionID int64, body string, mediaInputs []MediaInput) (*model.Answer, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if len(mediaInputs) > MaxMediaCount {
		return nil, apperr.InvalidInput(fmt.Sprintf("写真は%d枚まで添付できます", MaxMediaCount))
	}
	if err := validateBody(body); err != nil {
		return nil, err
	}
	prefix := fmt.Sprintf("media/%d/", claims.ID)
	for _, input := range mediaInputs {
		if !strings.HasPrefix(input.StorageKey, prefix) {
			return nil, fmt.Errorf("invalid media key")
		}
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

	now := time.Now()
	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		if err := uc.answerRepo.SaveAnswer(ctx, a); err != nil {
			return err
		}
		for i, input := range mediaInputs {
			media := &model.Media{
				UploaderUserID: claims.ID,
				StorageKey:     input.StorageKey,
				ContentType:    input.ContentType,
				CreatedAt:      now,
			}
			if err := uc.mediaRepo.CreateMedia(ctx, media); err != nil {
				return err
			}
			if err := uc.mediaRepo.CreateAnswerMedia(ctx, a.ID, media.ID, i); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return a, nil
}
