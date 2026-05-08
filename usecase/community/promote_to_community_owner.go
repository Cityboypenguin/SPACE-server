package community

import (
	"context"
	"errors"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type PromoteToCommunityOwnerUseCase interface {
	Execute(ctx context.Context, communityID, targetUserID int64) (bool, error)
}

var _ PromoteToCommunityOwnerUseCase = &PromoteToCommunityOwnerInteractor{}

type PromoteToCommunityOwnerInteractor struct {
	communityRepo repository.CommunityRepository
	roomUserRepo  repository.RoomUserRepository
}

func NewPromoteToCommunityOwnerUseCase(
	communityRepo repository.CommunityRepository,
	roomUserRepo repository.RoomUserRepository,
) PromoteToCommunityOwnerUseCase {
	return &PromoteToCommunityOwnerInteractor{
		communityRepo: communityRepo,
		roomUserRepo:  roomUserRepo,
	}
}

func (uc *PromoteToCommunityOwnerInteractor) Execute(ctx context.Context, communityID, targetUserID int64) (bool, error) {
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
			return false, errors.New("forbidden: only community owners or administrators can promote members")
		}
	}

	targetRole, err := uc.roomUserRepo.GetRoomUserRole(ctx, c.RoomID, targetUserID)
	if err != nil {
		return false, err
	}
	if targetRole == "" {
		return false, errors.New("user is not a member of this community")
	}

	if err := uc.roomUserRepo.SetRoomUserRole(ctx, c.RoomID, targetUserID, model.RoomUserRoleOwner); err != nil {
		return false, err
	}
	return true, nil
}
