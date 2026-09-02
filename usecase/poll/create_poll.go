package poll

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

type CreatePollUseCase interface {
	Execute(ctx context.Context, roomID int64, question string, optionLabels []string, allowMultipleChoice bool, deadline *time.Time) (*model.Poll, error)
}

var _ CreatePollUseCase = &CreatePollInteractor{}

type CreatePollInteractor struct {
	pollRepo        repository.PollRepository
	requireWritable course.RequireWritableCourseRoomUseCase
}

func NewCreatePollUseCase(pollRepo repository.PollRepository, requireWritable course.RequireWritableCourseRoomUseCase) CreatePollUseCase {
	return &CreatePollInteractor{pollRepo: pollRepo, requireWritable: requireWritable}
}

func (uc *CreatePollInteractor) Execute(ctx context.Context, roomID int64, question string, optionLabels []string, allowMultipleChoice bool, deadline *time.Time) (*model.Poll, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if len(optionLabels) < 2 {
		return nil, apperr.InvalidInput("選択肢は2つ以上必要です")
	}
	if deadline != nil && !deadline.After(time.Now()) {
		return nil, apperr.InvalidInput("回答期限は未来の日時を指定してください")
	}
	if _, err := uc.requireWritable.Execute(ctx, roomID); err != nil {
		return nil, err
	}

	return uc.pollRepo.CreatePoll(ctx, repository.CreatePollParam{
		RoomID:              roomID,
		AuthorUserID:        claims.ID,
		AuthorRole:          model.AuthorRoleStudent,
		Question:            question,
		AllowMultipleChoice: allowMultipleChoice,
		Deadline:            deadline,
		OptionLabels:        optionLabels,
	})
}
