package message

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdateMessageUseCase interface {
	Execute(ctx context.Context, messageID int64, updateParam model.UpdateMessageParam) (*model.Message, error)
}

var _ UpdateMessageUseCase = &UpdateMessageInteractor{}

type UpdateMessageInteractor struct {
	messageRepo repository.MessageRepository
}

func NewUpdateMessageUseCase(messageRepo repository.MessageRepository) UpdateMessageUseCase {
	return &UpdateMessageInteractor{messageRepo: messageRepo}
}

func (uc *UpdateMessageInteractor) Execute(ctx context.Context, messageID int64, updateParam model.UpdateMessageParam) (*model.Message, error) {
	message, err := uc.messageRepo.GetMessageByID(ctx, messageID)
	if err != nil {
		return nil, err
	}
	message.UpdateMessage(updateParam)
	if err := uc.messageRepo.UpdateMessage(ctx, message); err != nil {
		return nil, err
	}
	return message, nil
}
