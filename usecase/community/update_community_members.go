package community

import (
	"context"
	"errors"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MemberAction string

const (
	MemberActionPromote MemberAction = "promote"
	MemberActionDemote  MemberAction = "demote"
	MemberActionKick    MemberAction = "kick"
)

type MemberUpdate struct {
	UserID int64
	Action MemberAction
}

type UpdateCommunityMembersUseCase interface {
	Execute(ctx context.Context, communityID int64, updates []MemberUpdate) error
}

var _ UpdateCommunityMembersUseCase = &UpdateCommunityMembersInteractor{}

type UpdateCommunityMembersInteractor struct {
	communityRepo repository.CommunityRepository
	roomUserRepo  repository.RoomUserRepository
	txManager     repository.TxManager
}

func NewUpdateCommunityMembersUseCase(
	communityRepo repository.CommunityRepository,
	roomUserRepo repository.RoomUserRepository,
	txManager repository.TxManager,
) UpdateCommunityMembersUseCase {
	return &UpdateCommunityMembersInteractor{
		communityRepo: communityRepo,
		roomUserRepo:  roomUserRepo,
		txManager:     txManager,
	}
}

func (uc *UpdateCommunityMembersInteractor) Execute(ctx context.Context, communityID int64, updates []MemberUpdate) error {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return err
	}

	c, err := uc.communityRepo.GetCommunityByID(ctx, communityID)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("community not found")
	}

	if !authz.IsAdminRole(claims.Role) {
		callerRole, err := uc.roomUserRepo.GetRoomUserRole(ctx, c.RoomID, claims.ID)
		if err != nil {
			return err
		}
		if callerRole != model.RoomUserRoleOwner {
			return errors.New("forbidden: only community owners or administrators can update members")
		}
	}

	if err := uc.validateNoDuplicates(updates); err != nil {
		return err
	}

	members, err := uc.roomUserRepo.ListRoomMembersWithRoles(ctx, c.RoomID)
	if err != nil {
		return err
	}

	if err := uc.validateAndSimulate(members, updates); err != nil {
		return err
	}

	return uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		for _, u := range updates {
			switch u.Action {
			case MemberActionPromote:
				if err := uc.roomUserRepo.SetRoomUserRole(ctx, c.RoomID, u.UserID, model.RoomUserRoleOwner); err != nil {
					return err
				}
			case MemberActionDemote:
				if err := uc.roomUserRepo.SetRoomUserRole(ctx, c.RoomID, u.UserID, model.RoomUserRoleMember); err != nil {
					return err
				}
			case MemberActionKick:
				if err := uc.roomUserRepo.RemoveUserFromRoom(ctx, c.RoomID, u.UserID); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (uc *UpdateCommunityMembersInteractor) validateNoDuplicates(updates []MemberUpdate) error {
	seen := make(map[int64]struct{}, len(updates))
	for _, u := range updates {
		if _, dup := seen[u.UserID]; dup {
			return fmt.Errorf("duplicate update for user %d", u.UserID)
		}
		seen[u.UserID] = struct{}{}
	}
	return nil
}

// validateAndSimulate checks all target users are members and that the resulting
// state still has at least one owner.
func (uc *UpdateCommunityMembersInteractor) validateAndSimulate(members []*model.RoomMember, updates []MemberUpdate) error {
	roleMap := make(map[int64]string, len(members))
	for _, m := range members {
		roleMap[m.User.ID] = m.Role
	}

	updateMap := make(map[int64]MemberAction, len(updates))
	for _, u := range updates {
		if _, isMember := roleMap[u.UserID]; !isMember {
			return fmt.Errorf("user %d is not a member of this community", u.UserID)
		}
		updateMap[u.UserID] = u.Action
	}

	ownerCount := 0
	for userID, currentRole := range roleMap {
		resultRole := currentRole
		if action, ok := updateMap[userID]; ok {
			switch action {
			case MemberActionPromote:
				resultRole = model.RoomUserRoleOwner
			case MemberActionDemote:
				resultRole = model.RoomUserRoleMember
			case MemberActionKick:
				resultRole = ""
			}
		}
		if resultRole == model.RoomUserRoleOwner {
			ownerCount++
		}
	}

	if ownerCount == 0 {
		return errors.New("at least one owner must remain in the community")
	}
	return nil
}
