package message

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteMessageUseCase interface {
	// Execute soft-deletes messageID, recording deletedBy as the actor. It returns
	// false (with no error) if the message does not exist or was already deleted.
	Execute(ctx context.Context, messageID int64, deletedBy int64) (bool, error)
}

var _ DeleteMessageUseCase = &DeleteMessageInteractor{}

type DeleteMessageInteractor struct {
	messageRepo repository.MessageRepository
}

func NewDeleteMessageUseCase(messageRepo repository.MessageRepository) DeleteMessageUseCase {
	return &DeleteMessageInteractor{messageRepo: messageRepo}
}

func (uc *DeleteMessageInteractor) Execute(ctx context.Context, messageID int64, deletedBy int64) (bool, error) {
	return uc.messageRepo.SoftDeleteMessage(ctx, messageID, deletedBy)
}
