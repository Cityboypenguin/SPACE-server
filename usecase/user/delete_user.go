package user

import (
	"context"
	"errors"
	"sort"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteUserUseCase interface {
	Execute(ctx context.Context, id int64) (bool, error)
}

var _ DeleteUserUseCase = &DeleteUserInteractor{}

type DeleteUserInteractor struct {
	userRepo     userDeletionRepository
	postRepo     repository.PostWriter
	roomRepo     repository.RoomRepository
	roomUserRepo repository.RoomRoleRepository
	txManager    repository.TxManager
}

func NewDeleteUserUseCase(userRepo userDeletionRepository, postRepo repository.PostWriter, roomRepo repository.RoomRepository, roomUserRepo repository.RoomRoleRepository, txManager repository.TxManager) DeleteUserUseCase {
	return &DeleteUserInteractor{
		userRepo:     userRepo,
		postRepo:     postRepo,
		roomRepo:     roomRepo,
		roomUserRepo: roomUserRepo,
		txManager:    txManager,
	}
}

func (uc *DeleteUserInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	if _, err := authz.RequireSelfOrAdmin(ctx, id); err != nil {
		return false, err
	}
	var deleted bool
	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		memberships, err := uc.roomUserRepo.LockUserCommunityMembershipsForUpdate(ctx, id)
		if err != nil {
			return err
		}

		ownedRoomIDs := make([]int64, 0)
		for roomID, role := range memberships {
			if role == model.RoomUserRoleOwner {
				ownedRoomIDs = append(ownedRoomIDs, roomID)
			}
		}
		sort.Slice(ownedRoomIDs, func(i, j int) bool { return ownedRoomIDs[i] < ownedRoomIDs[j] })

		emptyCommunityRoomIDs := make([]int64, 0)
		for _, roomID := range ownedRoomIDs {
			roles, err := uc.roomUserRepo.LockRoomMemberRolesForUpdate(ctx, roomID)
			if err != nil {
				return err
			}
			ownerCount := 0
			for _, role := range roles {
				if role == model.RoomUserRoleOwner {
					ownerCount++
				}
			}
			if roles[id] == model.RoomUserRoleOwner && ownerCount == 1 && len(roles) > 1 {
				return errors.New("cannot delete user: they are the sole owner of a community with other members; transfer ownership first")
			}
			if len(roles) == 1 {
				emptyCommunityRoomIDs = append(emptyCommunityRoomIDs, roomID)
			}
		}

		if err := uc.postRepo.DeletePostsByUserID(ctx, id); err != nil {
			return err
		}
		for _, roomID := range emptyCommunityRoomIDs {
			if _, err := uc.roomRepo.DeleteRoom(ctx, roomID); err != nil {
				return err
			}
		}
		deleted, err = uc.userRepo.DeleteUser(ctx, id)
		if err != nil {
			return err
		}
		if !deleted {
			return nil
		}
		if err := uc.userRepo.DeleteActivityHistory(ctx, id); err != nil {
			return err
		}
		return uc.postRepo.RecalculateReplyCountsAffectedByUser(ctx, id)
	}); err != nil {
		return false, err
	}
	return deleted, nil
}
