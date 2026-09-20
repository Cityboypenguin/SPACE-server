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
	store repository.MessageReader
}

func NewGetMessageByIDUseCase(store repository.MessageReader) GetMessageByIDUseCase {
	return &GetMessageByIDInteractor{store: store}
}

func (uc *GetMessageByIDInteractor) Execute(ctx context.Context, messageID int64) (*model.Message, error) {
	return uc.store.GetMessageByID(ctx, messageID)
}
