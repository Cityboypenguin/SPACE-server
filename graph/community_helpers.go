package graph

import (
	"context"
	"log"

	"github.com/Cityboypenguin/SPACE-server/model"
)

func (r *mutationResolver) deleteCommunityIfEmpty(ctx context.Context, roomID int64) error {
	memberIDs, err := r.GetUserIDsByRoomIDUseCase.Execute(ctx, roomID)
	if err != nil {
		return err
	}
	if len(memberIDs) == 0 {
		if _, err := r.DeleteRoomUseCase.Execute(ctx, roomID); err != nil {
			log.Printf("failed to delete empty community room %d: %v", roomID, err)
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
