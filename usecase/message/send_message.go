package message

import (
	"context"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SendMessageUseCase interface {
	Execute(ctx context.Context, roomID, userID int64, content string, attachmentKey *string, attachmentName *string) (*model.Message, error)
}

var _ SendMessageUseCase = &SendMessageInteractor{}

type SendMessageInteractor struct {
	messageRepo repository.MessageRepository
}

func NewSendMessageUseCase(messageRepo repository.MessageRepository) SendMessageUseCase {
	return &SendMessageInteractor{messageRepo: messageRepo}
}

func (uc *SendMessageInteractor) Execute(ctx context.Context, roomID, userID int64, content string, attachmentKey *string, attachmentName *string) (*model.Message, error) {
	if content == "" && attachmentKey == nil {
		return nil, fmt.Errorf("content or attachment is required")
	}

	now := time.Now()
	m := &model.Message{}
	m.CreateMessage(model.CreateMessageParam{
		RoomID:         roomID,
		UserID:         userID,
		Content:        content,
		AttachmentKey:  attachmentKey,
		AttachmentName: attachmentName,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err := uc.messageRepo.SaveMessage(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}
