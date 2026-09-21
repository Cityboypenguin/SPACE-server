package room

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type LeaveCommunityUseCase interface {
	Execute(ctx context.Context, roomID int64) (bool, error)
}

type LeaveCommunityInteractor struct {
	roomRepo     repository.RoomRepository
	roomUserRepo leaveRoomRepository
	txManager    repository.TxManager
}

func NewLeaveCommunityUseCase(roomRepo repository.RoomRepository, roomUserRepo leaveRoomRepository, txManager repository.TxManager) LeaveCommunityUseCase {
	return &LeaveCommunityInteractor{roomRepo: roomRepo, roomUserRepo: roomUserRepo, txManager: txManager}
}

func (uc *LeaveCommunityInteractor) Execute(ctx context.Context, roomID int64) (bool, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return false, err
	}

	err = uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		room, err := uc.roomRepo.GetRoomByID(ctx, roomID)
		if err != nil {
			return err
		}
		if room == nil {
			return errors.New("room not found")
		}
		if room.Type != model.RoomTypeCommunity {
			return errors.New("can only leave community rooms")
		}

		roles, err := uc.roomUserRepo.LockRoomMemberRolesForUpdate(ctx, roomID)
		if err != nil {
			return err
		}
		leavingRole, isMember := roles[claims.ID]
		if !isMember {
			return errors.New("not a member of this community")
		}

		ownerCount := 0
		for _, role := range roles {
			if role == model.RoomUserRoleOwner {
				ownerCount++
			}
		}
		if leavingRole == model.RoomUserRoleOwner && ownerCount == 1 && len(roles) > 1 {
			return errors.New("cannot leave: you are the last owner; transfer ownership to another member first")
		}

		if len(roles) == 1 {
			deleted, err := uc.roomRepo.DeleteRoom(ctx, roomID)
			if err != nil {
				return err
			}
			if !deleted {
				return errors.New("room not found")
			}
			return nil
		}
		return uc.roomUserRepo.RemoveUserFromRoom(ctx, roomID, claims.ID)
	})
	if err != nil {
		return false, err
	}
	return true, nil
}
