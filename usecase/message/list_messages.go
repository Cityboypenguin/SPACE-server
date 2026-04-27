package message

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListMessagesUseCase interface {
	Execute(ctx context.Context, roomID int64) ([]*model.Message, error)
}

var _ ListMessagesUseCase = &ListMessagesInteractor{}

type ListMessagesInteractor struct {
	messageRepo repository.MessageRepository
}

func NewListMessagesUseCase(messageRepo repository.MessageRepository) ListMessagesUseCase {
	return &ListMessagesInteractor{messageRepo: messageRepo}
}

func (uc *ListMessagesInteractor) Execute(ctx context.Context, roomID int64) ([]*model.Message, error) {
	return uc.messageRepo.ListMessagesByRoomID(ctx, roomID)
}
