package question

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

type CreateQuestionUseCase interface {
	Execute(ctx context.Context, roomID int64, body string, mediaInputs []MediaInput) (*model.Question, error)
}

var _ CreateQuestionUseCase = &CreateQuestionInteractor{}

type CreateQuestionInteractor struct {
	questionRepo    repository.QuestionRepository
	mediaRepo       repository.MediaRepository
	txManager       repository.TxManager
	requireWritable course.RequireWritableCourseRoomUseCase
}

func NewCreateQuestionUseCase(
	questionRepo repository.QuestionRepository,
	mediaRepo repository.MediaRepository,
	txManager repository.TxManager,
	requireWritable course.RequireWritableCourseRoomUseCase,
) CreateQuestionUseCase {
	return &CreateQuestionInteractor{
		questionRepo:    questionRepo,
		mediaRepo:       mediaRepo,
		txManager:       txManager,
		requireWritable: requireWritable,
	}
}

func (uc *CreateQuestionInteractor) Execute(ctx context.Context, roomID int64, body string, mediaInputs []MediaInput) (*model.Question, error) {
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
	if _, err := uc.requireWritable.Execute(ctx, roomID); err != nil {
		return nil, err
	}

	q := &model.Question{
		RoomID:      roomID,
		AskerUserID: claims.ID,
		AuthorRole:  model.AuthorRoleStudent,
		Body:        body,
	}

	now := time.Now()
	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		if err := uc.questionRepo.SaveQuestion(ctx, q); err != nil {
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
			if err := uc.mediaRepo.CreateQuestionMedia(ctx, q.ID, media.ID, i); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return q, nil
}
