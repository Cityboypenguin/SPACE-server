package message

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetMessageByIDUseCase interface {
	Execute(ctx context.Context, messageID int64) (*model.Message, error)
}

var _ GetMessageByIDUseCase = &GetMessageByIDInteractor{}

type GetMessageByIDInteractor struct {
	messageRepo repository.MessageRepository
}

func NewGetMessageByIDUseCase(messageRepo repository.MessageRepository) GetMessageByIDUseCase {
	return &GetMessageByIDInteractor{messageRepo: messageRepo}
}

func (uc *GetMessageByIDInteractor) Execute(ctx context.Context, messageID int64) (*model.Message, error) {
	return uc.messageRepo.GetMessageByID(ctx, messageID)
}
