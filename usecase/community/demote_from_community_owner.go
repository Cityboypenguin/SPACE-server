package community

import (
	"context"
	"errors"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DemoteFromCommunityOwnerUseCase interface {
	Execute(ctx context.Context, communityID, targetUserID int64) (bool, error)
}

var _ DemoteFromCommunityOwnerUseCase = &DemoteFromCommunityOwnerInteractor{}

type DemoteFromCommunityOwnerInteractor struct {
	communityRepo repository.CommunityRepository
	roomUserRepo  repository.RoomUserRepository
}

func NewDemoteFromCommunityOwnerUseCase(
	communityRepo repository.CommunityRepository,
	roomUserRepo repository.RoomUserRepository,
) DemoteFromCommunityOwnerUseCase {
	return &DemoteFromCommunityOwnerInteractor{
		communityRepo: communityRepo,
		roomUserRepo:  roomUserRepo,
	}
}

func (uc *DemoteFromCommunityOwnerInteractor) Execute(ctx context.Context, communityID, targetUserID int64) (bool, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return false, err
	}

	c, err := uc.communityRepo.GetCommunityByID(ctx, communityID)
	if err != nil {
		return false, err
	}
	if c == nil {
		return false, fmt.Errorf("community not found")
	}

	if !authz.IsAdminRole(claims.Role) {
		callerRole, err := uc.roomUserRepo.GetRoomUserRole(ctx, c.RoomID, claims.ID)
		if err != nil {
			return false, err
		}
		if callerRole != model.RoomUserRoleOwner {
			return false, errors.New("forbidden: only community owners or administrators can demote owners")
		}
	}

	targetRole, err := uc.roomUserRepo.GetRoomUserRole(ctx, c.RoomID, targetUserID)
	if err != nil {
		return false, err
	}
	if targetRole == "" {
		return false, errors.New("user is not a member of this community")
	}
	if targetRole != model.RoomUserRoleOwner {
		return false, errors.New("user is not a community owner")
	}

	ownerCount, err := uc.roomUserRepo.CountRoomUsersByRole(ctx, c.RoomID, model.RoomUserRoleOwner)
	if err != nil {
		return false, err
	}
	if ownerCount <= 1 {
		return false, errors.New("cannot demote the last owner; promote another member first")
	}

	if err := uc.roomUserRepo.SetRoomUserRole(ctx, c.RoomID, targetUserID, model.RoomUserRoleMember); err != nil {
		return false, err
	}
	return true, nil
}
