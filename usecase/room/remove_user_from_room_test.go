package room

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type removeRoomRepo struct {
	repository.RoomRepository
	room *model.Room
}

func (r removeRoomRepo) GetRoomByID(context.Context, int64) (*model.Room, error) { return r.room, nil }

type recordingRoomUserRemove struct {
	repository.RoomUserRepository
	calls int
}

func (r *recordingRoomUserRemove) RemoveUserFromRoom(context.Context, int64, int64) error {
	r.calls++
	return nil
}

func TestRemoveUserFromRoomOnlyAllowsCommunities(t *testing.T) {
	for _, roomType := range []string{model.RoomTypeCourse, model.RoomTypeDM, "unknown"} {
		t.Run(roomType, func(t *testing.T) {
			users := &recordingRoomUserRemove{}
			uc := NewRemoveUserFromRoomUseCase(removeRoomRepo{room: &model.Room{Type: roomType}}, users)
			if err := uc.Execute(context.Background(), 1, 2); err == nil {
				t.Fatalf("room type %q must be rejected", roomType)
			}
			if users.calls != 0 {
				t.Fatalf("RemoveUserFromRoom called %d time(s)", users.calls)
			}
		})
	}

	users := &recordingRoomUserRemove{}
	uc := NewRemoveUserFromRoomUseCase(removeRoomRepo{room: &model.Room{Type: model.RoomTypeCommunity}}, users)
	if err := uc.Execute(context.Background(), 1, 2); err != nil {
		t.Fatalf("community removal failed: %v", err)
	}
	if users.calls != 1 {
		t.Fatalf("RemoveUserFromRoom calls = %d, want 1", users.calls)
	}
}
