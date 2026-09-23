package graph

import (
	"errors"
	"testing"
	"time"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/model"
)

// Message.user と Message.room はスキーマで非null（user: User! / room: Room!）。
//
// 非nullのフィールドに nil を返すと、gqlgen の非null用マーシャラが
// 「the requested element is null which the schema does not allow」を立てて
// 親ごと落とす。メッセージは [Message!]! に入って返るので、1件でもこうなると
// そのルームの履歴が丸ごと出なくなる。
//
// 「引けなかったので nil を返しておく」は、ここでは黙って隠すことにならず、
// 画面を消すことになる。その取り違えを繰り返さないための回帰テスト。

func dmRoom(id int64) *model.Room {
	return &model.Room{ID: id, Name: "DM", Type: model.RoomTypeDM, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}
}

func messageIn(roomID, userID int64) *gqlmodel.Message {
	return &gqlmodel.Message{
		ID:     encodeGraphID("message", 1),
		RoomID: encodeGraphID("room", roomID),
		UserID: encodeGraphID("user", userID),
	}
}

// 退会したユーザーのメッセージ。投稿側（postResolver.User）と同じく
// 「削除されたアカウント」の代替を返し、履歴は読めるままにする。
func TestMessageUser_DeletedUserFallsBackToPlaceholder(t *testing.T) {
	f := newLoaderFixture(t, fixtureOpts{
		rooms: map[int64]*model.Room{7: dmRoom(7)},
		users: map[int64]*model.User{}, // 退会済み＝行が無い
	})
	r := &messageResolver{&Resolver{}}

	user, err := r.User(f.ctx, messageIn(7, 42))
	if err != nil {
		t.Fatalf("退会ユーザーのメッセージで履歴が落ちている: %v", err)
	}
	if user == nil {
		t.Fatal("user が nil。非nullフィールドなので親ごと落ちる（履歴が出なくなる）")
	}
	if user.Name != deletedAccountDisplayName {
		t.Fatalf("表示名 = %q, want %q", user.Name, deletedAccountDisplayName)
	}
	if want := encodeGraphID("user", 42); user.ID != want {
		t.Fatalf("ID = %q, want %q", user.ID, want)
	}
}

// 読み込みエラーを代替で塗り潰さない。潰すと、DBが一時的に答えられないだけの
// 相手まで退会済みとして表示してしまう。
func TestMessageUser_LoadErrorIsNotHiddenAsDeleted(t *testing.T) {
	f := newLoaderFixture(t, fixtureOpts{
		rooms:   map[int64]*model.Room{7: dmRoom(7)},
		userErr: errors.New("boom"),
	})
	r := &messageResolver{&Resolver{}}

	user, err := r.User(f.ctx, messageIn(7, 42))
	if err == nil {
		t.Fatalf("読み込みエラーが握り潰されている（user=%v）", user)
	}
	if user != nil && user.Name == deletedAccountDisplayName {
		t.Fatal("読み込みエラーを退会済みとして表示している")
	}
}

// ルームが引けないのは、ON DELETE CASCADE のある構成では起こらないはずの
// データ異常。nil を返しても隠せない（親ごと落ちる）ので、そのまま返す。
func TestMessageRoom_MissingRoomReturnsError(t *testing.T) {
	f := newLoaderFixture(t, fixtureOpts{
		rooms: map[int64]*model.Room{}, // 引けない
	})
	r := &messageResolver{&Resolver{}}

	room, err := r.Room(f.ctx, messageIn(7, 42))
	if err == nil {
		t.Fatalf("ルームが引けないのにエラーにならない（room=%v）。nil を返しても非nullなので親ごと落ちる", room)
	}
}

func TestMessageRoom_PresentRoomIsReturned(t *testing.T) {
	f := newLoaderFixture(t, fixtureOpts{
		rooms: map[int64]*model.Room{7: dmRoom(7)},
	})
	r := &messageResolver{&Resolver{}}

	room, err := r.Room(f.ctx, messageIn(7, 42))
	if err != nil {
		t.Fatalf("ルームが引けない: %v", err)
	}
	if room == nil {
		t.Fatal("room が nil")
	}
}
