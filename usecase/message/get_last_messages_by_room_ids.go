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
	readModel repository.MessageReadModel
}

func NewGetLastMessagesByRoomIDsUseCase(readModel repository.MessageReadModel) GetLastMessagesByRoomIDsUseCase {
	return &GetLastMessagesByRoomIDsInteractor{readModel: readModel}
}

func (uc *GetLastMessagesByRoomIDsInteractor) Execute(ctx context.Context, roomIDs []int64) (map[int64]*model.Message, error) {
	return uc.readModel.GetLastMessagesByRoomIDs(ctx, roomIDs)
}
