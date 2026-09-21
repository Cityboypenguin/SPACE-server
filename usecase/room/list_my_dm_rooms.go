package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListMyDMRoomsUseCase interface {
	Execute(ctx context.Context, q repository.PageQuery) ([]*model.Room, int, error)
}

var _ ListMyDMRoomsUseCase = &ListMyDMRoomsInteractor{}

type ListMyDMRoomsInteractor struct {
	roomUserRepo repository.DMRoomRepository
}

func NewListMyDMRoomsUseCase(roomUserRepo repository.DMRoomRepository) ListMyDMRoomsUseCase {
	return &ListMyDMRoomsInteractor{roomUserRepo: roomUserRepo}
}

func (uc *ListMyDMRoomsInteractor) Execute(ctx context.Context, q repository.PageQuery) ([]*model.Room, int, error) {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return nil, 0, err
	}
	return uc.roomUserRepo.ListDMRoomsByUserID(ctx, userID, q)
}
