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

	// 操作の種類ごとにまとめてから撃つ。以前は updates を1件ずつ回して人数ぶんの
	// UPDATE / DELETE を出しており、20人チェックすれば20往復していた
	// （トランザクションの中なので結果は同じで、費用だけが人数に比例していた）。
	// 種類は3つしか無いので、まとめれば最大3本で済む。
	//
	// 同じ利用者が2回出てこないことは validateNoDuplicates が先に保証しているので、
	// 「昇格と除名が同じ人に当たって順序で結果が変わる」は起きない。
	promote, demote, kick := groupByAction(updates)

	return uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		if len(promote) > 0 {
			if err := uc.roomUserRepo.SetRoomUserRoles(ctx, c.RoomID, promote, model.RoomUserRoleOwner); err != nil {
				return err
			}
		}
		if len(demote) > 0 {
			if err := uc.roomUserRepo.SetRoomUserRoles(ctx, c.RoomID, demote, model.RoomUserRoleMember); err != nil {
				return err
			}
		}
		if len(kick) > 0 {
			if err := uc.roomUserRepo.RemoveUsersFromRoom(ctx, c.RoomID, kick); err != nil {
				return err
			}
		}
		return nil
	})
}

// groupByAction は更新指定を操作の種類ごとの利用者IDへ振り分ける。
// 知らない Action は黙って落とす（1件ずつ回していたときの switch に
// default が無かったのと同じ扱い）。
func groupByAction(updates []MemberUpdate) (promote, demote, kick []int64) {
	for _, u := range updates {
		switch u.Action {
		case MemberActionPromote:
			promote = append(promote, u.UserID)
		case MemberActionDemote:
			demote = append(demote, u.UserID)
		case MemberActionKick:
			kick = append(kick, u.UserID)
		}
	}
	return promote, demote, kick
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
