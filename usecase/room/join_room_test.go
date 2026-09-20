package room

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type joinRoomRepo struct {
	repository.RoomRepository
	room *model.Room
}

func (r joinRoomRepo) GetRoomByID(context.Context, int64) (*model.Room, error) { return r.room, nil }

type recordingRoomUserJoin struct {
	repository.RoomUserRepository
	calls  int
	userID int64
}

func (r *recordingRoomUserJoin) AddUserToRoom(_ context.Context, _ int64, userID int64) error {
	r.calls++
	r.userID = userID
	return nil
}

func TestJoinRoomOnlyAllowsCommunities(t *testing.T) {
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 42})
	for _, roomType := range []string{model.RoomTypeCourse, model.RoomTypeDM, "unknown"} {
		t.Run(roomType, func(t *testing.T) {
			users := &recordingRoomUserJoin{}
			uc := NewJoinRoomUseCase(joinRoomRepo{room: &model.Room{Type: roomType}}, users)
			if _, err := uc.Execute(ctx, 1); err == nil {
				t.Fatalf("room type %q must be rejected", roomType)
			}
			if users.calls != 0 {
				t.Fatalf("AddUserToRoom called %d time(s)", users.calls)
			}
		})
	}

	users := &recordingRoomUserJoin{}
	uc := NewJoinRoomUseCase(joinRoomRepo{room: &model.Room{Type: model.RoomTypeCommunity}}, users)
	ok, err := uc.Execute(ctx, 1)
	if err != nil || !ok {
		t.Fatalf("community join failed: ok=%v err=%v", ok, err)
	}
	if users.calls != 1 || users.userID != 42 {
		t.Fatalf("AddUserToRoom calls=%d userID=%d, want 1 and 42", users.calls, users.userID)
	}
}
