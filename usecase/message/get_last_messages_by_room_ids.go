package message

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetLastMessagesByRoomIDsUseCase interface {
	Execute(ctx context.Context, roomIDs []int64) (map[int64]*model.Message, error)
}

var _ GetLastMessagesByRoomIDsUseCase = &GetLastMessagesByRoomIDsInteractor{}

type GetLastMessagesByRoomIDsInteractor struct {
	messageRepo repository.MessageRepository
}

func NewGetLastMessagesByRoomIDsUseCase(messageRepo repository.MessageRepository) GetLastMessagesByRoomIDsUseCase {
	return &GetLastMessagesByRoomIDsInteractor{messageRepo: messageRepo}
}

func (uc *GetLastMessagesByRoomIDsInteractor) Execute(ctx context.Context, roomIDs []int64) (map[int64]*model.Message, error) {
	return uc.messageRepo.GetLastMessagesByRoomIDs(ctx, roomIDs)
}
