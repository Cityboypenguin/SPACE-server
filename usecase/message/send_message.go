package message

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SendMessageUseCase interface {
	Execute(ctx context.Context, roomID, userID int64, content string) (*model.Message, error)
}

var _ SendMessageUseCase = &SendMessageInteractor{}

type SendMessageInteractor struct {
	messageRepo repository.MessageRepository
}

func NewSendMessageUseCase(messageRepo repository.MessageRepository) SendMessageUseCase {
	return &SendMessageInteractor{messageRepo: messageRepo}
}

func (uc *SendMessageInteractor) Execute(ctx context.Context, roomID, userID int64, content string) (*model.Message, error) {
	now := time.Now()
	m := &model.Message{}
	m.CreateMessage(model.CreateMessageParam{
		RoomID:    roomID,
		UserID:    userID,
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err := uc.messageRepo.SaveMessage(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}
