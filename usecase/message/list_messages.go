package message

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListMessagesUseCase interface {
	Execute(ctx context.Context, roomID int64, limit int, beforeID *int64, afterID *int64, afterTime *time.Time) ([]*model.Message, bool, bool, error)
}

var _ ListMessagesUseCase = &ListMessagesInteractor{}

type ListMessagesInteractor struct {
	messageRepo repository.MessageRepository
}

func NewListMessagesUseCase(messageRepo repository.MessageRepository) ListMessagesUseCase {
	return &ListMessagesInteractor{messageRepo: messageRepo}
}

func (uc *ListMessagesInteractor) Execute(ctx context.Context, roomID int64, limit int, beforeID *int64, afterID *int64, afterTime *time.Time) ([]*model.Message, bool, bool, error) {
	return uc.messageRepo.ListMessagesByRoomID(ctx, roomID, limit, beforeID, afterID, afterTime)
}
