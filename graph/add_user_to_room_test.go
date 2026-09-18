package graph

import (
	"context"
	"testing"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
)

type recordingJoinRoom struct {
	calls  int
	roomID int64
}

func (j *recordingJoinRoom) Execute(_ context.Context, roomID int64) (bool, error) {
	j.calls++
	j.roomID = roomID
	return true, nil
}

func addUserToRoomInput(roomID, userID int64) gqlmodel.AddUserToRoomInput {
	return gqlmodel.AddUserToRoomInput{
		RoomID: encodeGraphID("room", roomID),
		UserID: encodeGraphID("user", userID),
	}
}

func TestAddUserToRoomDelegatesSelfJoinToUseCase(t *testing.T) {
	joiner := &recordingJoinRoom{}
	r := &Resolver{MessageRoomUseCases: MessageRoomUseCases{JoinRoomUseCase: joiner}}
	ctx := auth.WithClaims(context.Background(), userClaims(10))

	ok, err := (&mutationResolver{r}).AddUserToRoom(ctx, addUserToRoomInput(3, 10))
	if err != nil || !ok {
		t.Fatalf("self join failed: ok=%v err=%v", ok, err)
	}
	if joiner.calls != 1 || joiner.roomID != 3 {
		t.Fatalf("JoinRoom calls=%d roomID=%d, want 1 and 3", joiner.calls, joiner.roomID)
	}
}

func TestAddUserToRoomRejectsAddingAnotherUser(t *testing.T) {
	joiner := &recordingJoinRoom{}
	r := &Resolver{MessageRoomUseCases: MessageRoomUseCases{JoinRoomUseCase: joiner}}
	ctx := auth.WithClaims(context.Background(), userClaims(10))

	if _, err := (&mutationResolver{r}).AddUserToRoom(ctx, addUserToRoomInput(3, 99)); err == nil {
		t.Fatal("expected adding another user to be rejected")
	}
	if joiner.calls != 0 {
		t.Fatalf("JoinRoom calls=%d, want 0", joiner.calls)
	}
}
