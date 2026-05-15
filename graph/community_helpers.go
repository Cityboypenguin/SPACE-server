package graph

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
)

// promoteNewOwnerIfNeeded promotes the earliest-joined non-leaving member to owner
// if the leaving user is the last owner and other members remain.
func (r *mutationResolver) promoteNewOwnerIfNeeded(ctx context.Context, roomID, leavingUserID int64) error {
	members, err := r.ListRoomMembersWithRolesUseCase.Execute(ctx, roomID)
	if err != nil {
		return err
	}

	leavingIsOwner := false
	ownerCount := 0
	for _, m := range members {
		if m.Role == model.RoomUserRoleOwner {
			ownerCount++
			if m.User.ID == leavingUserID {
				leavingIsOwner = true
			}
		}
	}

	if !leavingIsOwner || ownerCount > 1 {
		return nil
	}

	// Find another member to promote (first non-leaving member)
	for _, m := range members {
		if m.User.ID != leavingUserID {
			if err := r.SetRoomUserRoleUseCase.Execute(ctx, roomID, m.User.ID, model.RoomUserRoleOwner); err != nil {
				return err
			}
			logger.Log.Info().
				Int64("room_id", roomID).
				Int64("new_owner_id", m.User.ID).
				Int64("leaving_user_id", leavingUserID).
				Msg("auto-promoted member to owner on owner leave")
			return nil
		}
	}

	// No other members — allow leave (community will be auto-deleted)
	return nil
}

func (r *mutationResolver) deleteCommunityIfEmpty(ctx context.Context, roomID int64) error {
	memberIDs, err := r.GetUserIDsByRoomIDUseCase.Execute(ctx, roomID)
	if err != nil {
		return err
	}
	if len(memberIDs) == 0 {
		if _, err := r.DeleteRoomUseCase.Execute(ctx, roomID); err != nil {
			logger.Log.Error().Err(err).Int64("room_id", roomID).Msg("failed to delete empty community room")
			return err
		}
	}
	return nil
}

func (r *mutationResolver) countCommunityOwners(ctx context.Context, roomID int64) (int, error) {
	members, err := r.ListRoomMembersWithRolesUseCase.Execute(ctx, roomID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, m := range members {
		if m.Role == model.RoomUserRoleOwner {
			count++
		}
	}
	return count, nil
}
