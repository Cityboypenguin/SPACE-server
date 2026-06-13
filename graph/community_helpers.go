package graph

import (
	"context"
	"errors"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
)

// checkCanLeaveCommunity returns an error if the leaving user is the sole owner
// while other members remain, preventing an ownerless community.
func (r *mutationResolver) checkCanLeaveCommunity(ctx context.Context, roomID, leavingUserID int64) error {
	members, err := r.ListRoomMembersWithRolesUseCase.Execute(ctx, roomID)
	if err != nil {
		return err
	}

	leavingIsOwner := false
	ownerCount := 0
	hasOtherMembers := false
	for _, m := range members {
		if m.Role == model.RoomUserRoleOwner {
			ownerCount++
			if m.User.ID == leavingUserID {
				leavingIsOwner = true
			}
		}
		if m.User.ID != leavingUserID {
			hasOtherMembers = true
		}
	}

	if leavingIsOwner && ownerCount == 1 && hasOtherMembers {
		return errors.New("cannot leave: you are the last owner; transfer ownership to another member first")
	}
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

func (r *Resolver) hydrateCommunityMembership(ctx context.Context, community *gqlmodel.Community, roomID int64, viewerUserID *int64) error {
	memberIDs, err := r.GetUserIDsByRoomIDUseCase.Execute(ctx, roomID)
	if err != nil {
		return err
	}

	community.MemberCount = int32(len(memberIDs))
	if viewerUserID != nil {
		community.IsMember = containsInt64(memberIDs, *viewerUserID)
	}
	return nil
}

func (r *Resolver) toGraphCommunityWithMembership(ctx context.Context, c *model.Community, viewerUserID *int64) (*gqlmodel.Community, error) {
	gqlCommunity := toGraphCommunity(c, r.communityAvatarURL(c))
	if gqlCommunity == nil {
		return nil, nil
	}
	if err := r.hydrateCommunityMembership(ctx, gqlCommunity, c.RoomID, viewerUserID); err != nil {
		return nil, err
	}
	return gqlCommunity, nil
}
