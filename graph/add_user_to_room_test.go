package graph

import (
	"context"
	"testing"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
)

// ■ addUserToRoom がルーム種別ごとの前提を壊せないこと
//
// このミューテーションは以前、コミュニティ以外では「呼び出し元がそのルームの
// メンバーか」しか見ておらず、任意のユーザーを room_users へ入れられた。
// 壊れるのは認可だけではなく、他のコードが置いている前提のほう:
//
//   - 授業ルームは参加者を時間割（履修）から決めるので、room_users に人を
//     置かれると履修していない人が混ざる。
//   - DM が2人であることは dmPartnerID（graph/helpers.go）が前提にしており、
//     3人目が入ると「相手1人」が決まらず false になって、ブロック判定が
//     黙ってスキップされる。

// stubGetRoom は指定のルームを返すだけの GetRoomUseCase。
type stubGetRoom struct{ room *model.Room }

func (s stubGetRoom) Execute(context.Context, int64) (*model.Room, error) { return s.room, nil }

// recordingJoinRoom は制約を持つ JoinRoom usecase が呼ばれたかを記録する。
type recordingJoinRoom struct{ calls int }

func (a *recordingJoinRoom) Execute(context.Context, int64) (bool, error) {
	a.calls++
	return true, nil
}

// stubRoomMembers は呼び出し元をメンバーとして返す。拒否が「メンバーでないから」
// ではなくルーム種別によるものだと分かるように、あえてメンバー扱いにしておく。
type stubRoomMembers struct{ ids []int64 }

func (s stubRoomMembers) Execute(context.Context, int64) ([]int64, error) { return s.ids, nil }

func addUserToRoomFixture(roomType string, callerID int64) (*Resolver, *recordingJoinRoom, context.Context) {
	joiner := &recordingJoinRoom{}
	r := &Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetRoomUseCase:            stubGetRoom{room: &model.Room{ID: 1, Type: roomType}},
			JoinRoomUseCase:           joiner,
			GetUserIDsByRoomIDUseCase: stubRoomMembers{ids: []int64{callerID}},
		},
	}
	ctx := auth.WithClaims(context.Background(), userClaims(callerID))
	return r, joiner, ctx
}

func addUserToRoomInput(roomID, userID int64) gqlmodel.AddUserToRoomInput {
	return gqlmodel.AddUserToRoomInput{
		RoomID: encodeGraphID("room", roomID),
		UserID: encodeGraphID("user", userID),
	}
}

// 授業ルームへの追加は、呼び出し元がメンバーでも拒否すること。
// 参加は時間割で決まるので、room_users へ入れる口そのものを開けない。
func TestAddUserToRoom_RejectsCourseRooms(t *testing.T) {
	r, adder, ctx := addUserToRoomFixture(model.RoomTypeCourse, 10)

	ok, err := (&mutationResolver{r}).AddUserToRoom(ctx, addUserToRoomInput(1, 99))
	if err == nil {
		t.Fatal("expected adding a user to a course room to be refused")
	}
	if ok {
		t.Fatal("ok = true, want false")
	}
	if adder.calls != 0 {
		t.Fatalf("room_users was written %d time(s); course membership must come from the timetable", adder.calls)
	}
}

// 自分自身であっても授業ルームには入れないこと（履修を迂回した自己参加も同じ穴）。
func TestAddUserToRoom_RejectsCourseRoomsEvenForSelf(t *testing.T) {
	r, adder, ctx := addUserToRoomFixture(model.RoomTypeCourse, 10)

	if _, err := (&mutationResolver{r}).AddUserToRoom(ctx, addUserToRoomInput(1, 10)); err == nil {
		t.Fatal("expected self-join to a course room to be refused")
	}
	if adder.calls != 0 {
		t.Fatalf("room_users was written %d time(s) for a course room", adder.calls)
	}
}

// DM への追加は拒否すること。3人目が入ると dmPartnerID が相手を決められず、
// ブロック判定が黙って効かなくなる。
func TestAddUserToRoom_RejectsDirectMessageRooms(t *testing.T) {
	r, adder, ctx := addUserToRoomFixture(model.RoomTypeDM, 10)

	ok, err := (&mutationResolver{r}).AddUserToRoom(ctx, addUserToRoomInput(1, 99))
	if err == nil {
		t.Fatal("expected adding a third user to a DM to be refused")
	}
	if ok {
		t.Fatal("ok = true, want false")
	}
	if adder.calls != 0 {
		t.Fatalf("room_users was written %d time(s); a DM must stay at two members", adder.calls)
	}
}

// コミュニティへの自己参加は従来どおり通ること。
func TestAddUserToRoom_AllowsJoiningACommunityAsYourself(t *testing.T) {
	r, adder, ctx := addUserToRoomFixture(model.RoomTypeCommunity, 10)

	ok, err := (&mutationResolver{r}).AddUserToRoom(ctx, addUserToRoomInput(1, 10))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if adder.calls != 1 {
		t.Fatalf("JoinRoom calls = %d, want 1", adder.calls)
	}
}

// コミュニティに他人を入れるのは従来どおり拒否すること。
func TestAddUserToRoom_RejectsAddingSomeoneElseToACommunity(t *testing.T) {
	r, adder, ctx := addUserToRoomFixture(model.RoomTypeCommunity, 10)

	if _, err := (&mutationResolver{r}).AddUserToRoom(ctx, addUserToRoomInput(1, 99)); err == nil {
		t.Fatal("expected adding someone else to a community to be refused")
	}
	if adder.calls != 0 {
		t.Fatalf("room_users was written %d time(s) for someone else", adder.calls)
	}
}

// 知らないルーム種別が来たら既定で拒否すること
// （種別を足した人が明示的に判断しないと開かないようにするため）。
func TestAddUserToRoom_RejectsUnknownRoomTypes(t *testing.T) {
	r, adder, ctx := addUserToRoomFixture("something_new", 10)

	if _, err := (&mutationResolver{r}).AddUserToRoom(ctx, addUserToRoomInput(1, 10)); err == nil {
		t.Fatal("an unknown room type must default to refusing the write")
	}
	if adder.calls != 0 {
		t.Fatalf("room_users was written %d time(s) for an unknown room type", adder.calls)
	}
}
