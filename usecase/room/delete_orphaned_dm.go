package room

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteOrphanedDMUseCase interface {
	Execute(ctx context.Context, roomID int64) (bool, error)
}

type DeleteOrphanedDMInteractor struct {
	roomRepo     repository.RoomRepository
	roomUserRepo repository.RoomRoleRepository
	txManager    repository.TxManager
}

func NewDeleteOrphanedDMUseCase(roomRepo repository.RoomRepository, roomUserRepo repository.RoomRoleRepository, txManager repository.TxManager) DeleteOrphanedDMUseCase {
	return &DeleteOrphanedDMInteractor{roomRepo: roomRepo, roomUserRepo: roomUserRepo, txManager: txManager}
}

func (uc *DeleteOrphanedDMInteractor) Execute(ctx context.Context, roomID int64) (bool, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return false, err
	}

	var deleted bool
	err = uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		room, err := uc.roomRepo.GetRoomByID(ctx, roomID)
		if err != nil {
			return err
		}
		if room == nil {
			return errors.New("room not found")
		}
		if room.Type != model.RoomTypeDM {
			return errors.New("forbidden: only DM rooms can be deleted")
		}

		roles, err := uc.roomUserRepo.LockRoomMemberRolesForUpdate(ctx, roomID)
		if err != nil {
			return err
		}
		if _, isMember := roles[claims.ID]; !isMember {
			return errors.New("forbidden: not a member of this room")
		}
		if len(roles) != 1 {
			return errors.New("forbidden: cannot delete a room with an active partner")
		}

		deleted, err = uc.roomRepo.DeleteRoom(ctx, roomID)
		return err
	})
	return deleted, err
}
