package graph

import (
	"context"
	"errors"
	"testing"
	"time"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/dataloader"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/vikstrous/dataloadgen"
)

// 授業内チャットの匿名表示（F-05）の回帰テスト。
//
// ここが壊れると「匿名のはずの投稿者が実名で出る」という、画面を見ても
// 正常に見えてしまう事故になる。DataLoader を挟んだことで壊れていないことを、
// リゾルバの入口から確かめる。
//
// 守りたい性質は2つ。
//   - 匿名IDの行が引けないときに実名へ落ちないこと（番号なしの「匿名」へ倒す）。
//   - userID と user の両方を解決しても同じ匿名IDになること。

// loaderFixture は匿名表示に関わる3つのローダーだけを差し込んだコンテキストを作る。
// 使わないローダーは nil のまま（触れば panic するので、経路が変わればすぐ分かる）。
type loaderFixture struct {
	ctx context.Context

	roomFetches     int
	identityFetches int
	userFetches     int
}

type fixtureOpts struct {
	rooms      map[int64]*model.Room
	identities map[repository.RoomUserKey]*model.RoomAnonymousIdentity
	users      map[int64]*model.User
	roomErr    error
	anonErr    error
}

func newLoaderFixture(t *testing.T, opts fixtureOpts) *loaderFixture {
	t.Helper()
	f := &loaderFixture{}

	// wait を 0 にして、テストが1件ずつ Load してもバッチ待ちで遅くならないようにする。
	// 待ち時間は挙動ではなく速度の設定なので、ここを変えても確かめたい性質は変わらない。
	const wait = 0

	roomLoader := dataloadgen.NewLoader(func(_ context.Context, ids []int64) ([]*model.Room, []error) {
		f.roomFetches++
		errs := make([]error, len(ids))
		out := make([]*model.Room, len(ids))
		if opts.roomErr != nil {
			for i := range errs {
				errs[i] = opts.roomErr
			}
			return out, errs
		}
		for i, id := range ids {
			out[i] = opts.rooms[id]
		}
		return out, errs
	}, dataloadgen.WithWait(wait))

	identityLoader := dataloadgen.NewLoader(func(_ context.Context, keys []repository.RoomUserKey) ([]*model.RoomAnonymousIdentity, []error) {
		f.identityFetches++
		errs := make([]error, len(keys))
		out := make([]*model.RoomAnonymousIdentity, len(keys))
		if opts.anonErr != nil {
			for i := range errs {
				errs[i] = opts.anonErr
			}
			return out, errs
		}
		for i, k := range keys {
			out[i] = opts.identities[k]
		}
		return out, errs
	}, dataloadgen.WithWait(wait))

	userLoader := dataloadgen.NewLoader(func(_ context.Context, ids []int64) ([]*model.User, []error) {
		f.userFetches++
		errs := make([]error, len(ids))
		out := make([]*model.User, len(ids))
		for i, id := range ids {
			out[i] = opts.users[id]
		}
		return out, errs
	}, dataloadgen.WithWait(wait))

	f.ctx = dataloader.WithLoaders(context.Background(), &dataloader.Loaders{
		RoomLoader:              roomLoader,
		AnonymousIdentityLoader: identityLoader,
		UserLoader:              userLoader,
	})
	return f
}

func courseRoom(id int64) *model.Room {
	return &model.Room{ID: id, Name: "授業", Type: model.RoomTypeCourse, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}
}

func realUser(id int64, name string) *model.User {
	return &model.User{ID: id, AccountID: "acct", Name: name, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}
}

// TestAnonymousUserForCourseRoom_NeverFallsBackToTheRealName は、匿名IDの行が
// 引けない／引くのに失敗した場合でも実名に落ちないことを確かめる。
//
// ここで nil が返ると呼び出し側は「授業ルームではない」と解釈して実名を出す。
// つまり nil は匿名性が壊れた状態であり、番号なしの「匿名」が正しい倒し方。
func TestAnonymousUserForCourseRoom_NeverFallsBackToTheRealName(t *testing.T) {
	const (
		roomID     = int64(7)
		authorID   = int64(42)
		authorName = "山田太郎"
	)
	roomGraphID := encodeGraphID("room", roomID)

	tests := []struct {
		name string
		opts fixtureOpts
	}{
		{
			name: "匿名IDの行がまだ無い（採番前の古いデータ）",
			opts: fixtureOpts{
				rooms:      map[int64]*model.Room{roomID: courseRoom(roomID)},
				identities: map[repository.RoomUserKey]*model.RoomAnonymousIdentity{},
				users:      map[int64]*model.User{authorID: realUser(authorID, authorName)},
			},
		},
		{
			name: "匿名IDの取得が失敗した（DB障害）",
			opts: fixtureOpts{
				rooms:   map[int64]*model.Room{roomID: courseRoom(roomID)},
				anonErr: errors.New("db is down"),
				users:   map[int64]*model.User{authorID: realUser(authorID, authorName)},
			},
		},
		// ルームが引けないと種別が確かめられない。「授業ルームではない」と
		// 同じ扱いにすると、授業ルームの投稿者が実名で出る。
		{
			name: "ルームの取得が失敗した（DB障害）",
			opts: fixtureOpts{
				roomErr:    errors.New("db is down"),
				identities: map[repository.RoomUserKey]*model.RoomAnonymousIdentity{},
				users:      map[int64]*model.User{authorID: realUser(authorID, authorName)},
			},
		},
		{
			name: "ルームの行が引けなかった",
			opts: fixtureOpts{
				rooms:      map[int64]*model.Room{},
				identities: map[repository.RoomUserKey]*model.RoomAnonymousIdentity{},
				users:      map[int64]*model.User{authorID: realUser(authorID, authorName)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newLoaderFixture(t, tt.opts)
			r := &Resolver{}

			got := r.anonymousUserForCourseRoom(f.ctx, roomGraphID, authorID)
			if got == nil {
				t.Fatal("nil を返すと呼び出し側が実名を出してしまう。番号なしの匿名を返すこと")
			}
			if got.Name != anonymousPlaceholderLabel {
				t.Fatalf("name = %q, want %q", got.Name, anonymousPlaceholderLabel)
			}
			if got.Name == authorName {
				t.Fatal("実名が漏れている")
			}
			// ID も実ユーザーのものであってはならない（他の画面と突き合わせられる）。
			if got.ID == encodeGraphID("user", authorID) {
				t.Fatal("実ユーザーのIDが漏れている")
			}
			// email は gqlmodel.User から消えたので、ここで見るのは accountID だけ
			// （連絡先が出ないことは型で保証されている。graph/schema.graphqls の
			//  User / UserAccount のコメント参照）。
			if got.AccountID != "" {
				t.Fatalf("匿名表示にアカウント情報が乗っている: %+v", got)
			}
		})
	}
}

// TestAnonymousUserForCourseRoom_ReturnsTheIdentityLabel は、行があるときは
// その匿名ラベルを返すことを確かめる（上のテストが常に匿名を返すだけの実装でも
// 通ってしまわないように）。
func TestAnonymousUserForCourseRoom_ReturnsTheIdentityLabel(t *testing.T) {
	const (
		roomID   = int64(7)
		authorID = int64(42)
	)
	identity := &model.RoomAnonymousIdentity{ID: 3, RoomID: roomID, UserID: authorID, Label: "匿名003", CreatedAt: time.Unix(0, 0)}

	f := newLoaderFixture(t, fixtureOpts{
		rooms:      map[int64]*model.Room{roomID: courseRoom(roomID)},
		identities: map[repository.RoomUserKey]*model.RoomAnonymousIdentity{{RoomID: roomID, UserID: authorID}: identity},
		users:      map[int64]*model.User{authorID: realUser(authorID, "山田太郎")},
	})

	got := (&Resolver{}).anonymousUserForCourseRoom(f.ctx, encodeGraphID("room", roomID), authorID)
	if got == nil {
		t.Fatal("expected the anonymous user")
	}
	if got.Name != "匿名003" {
		t.Fatalf("name = %q, want 匿名003", got.Name)
	}
	if got.ID != encodeGraphID("anon", identity.ID) {
		t.Fatal("匿名IDは identity 行から導くこと（実ユーザーIDと相関させない）")
	}
}

// TestAnonymousUserForCourseRoom_NonCourseRoomShowsTheRealUser は、授業ルーム以外は
// 従来どおり実名で出ることを確かめる（匿名化を広げすぎていないこと）。
func TestAnonymousUserForCourseRoom_NonCourseRoomShowsTheRealUser(t *testing.T) {
	const roomID = int64(7)
	f := newLoaderFixture(t, fixtureOpts{
		rooms: map[int64]*model.Room{roomID: {ID: roomID, Type: model.RoomTypeDM}},
	})

	if got := (&Resolver{}).anonymousUserForCourseRoom(f.ctx, encodeGraphID("room", roomID), 42); got != nil {
		t.Fatalf("DM で匿名化してはいけない: %+v", got)
	}
}

// TestMessageUserAndUserID_AgreeAndResolveOnce は項目7の本体。
//
// Message は userID と user の2フィールドが同じ匿名解決を通る。以前は別々に
// 引いていたので、同じメッセージで2回クエリが走っていた。DataLoader の
// リクエスト内キャッシュで1回に畳まれ、かつ必ず同じ匿名IDになること
// （別々に引くと、途中で採番されたときに2つのフィールドが食い違う）。
func TestMessageUserAndUserID_AgreeAndResolveOnce(t *testing.T) {
	const (
		roomID   = int64(7)
		authorID = int64(42)
	)
	identity := &model.RoomAnonymousIdentity{ID: 5, RoomID: roomID, UserID: authorID, Label: "匿名005", CreatedAt: time.Unix(0, 0)}

	f := newLoaderFixture(t, fixtureOpts{
		rooms:      map[int64]*model.Room{roomID: courseRoom(roomID)},
		identities: map[repository.RoomUserKey]*model.RoomAnonymousIdentity{{RoomID: roomID, UserID: authorID}: identity},
		users:      map[int64]*model.User{authorID: realUser(authorID, "山田太郎")},
	})

	mr := &messageResolver{&Resolver{}}
	msg := &gqlmodel.Message{
		ID:     encodeGraphID("message", 100),
		RoomID: encodeGraphID("room", roomID),
		UserID: encodeGraphID("user", authorID),
	}

	gotUserID, err := mr.UserID(f.ctx, msg)
	if err != nil {
		t.Fatalf("UserID: %v", err)
	}
	gotUser, err := mr.User(f.ctx, msg)
	if err != nil {
		t.Fatalf("User: %v", err)
	}
	if gotUser == nil {
		t.Fatal("User must not be nil in a course room")
	}

	if gotUserID != gotUser.ID {
		t.Fatalf("userID = %q but user.id = %q; 2つのフィールドが違う匿名IDを返している", gotUserID, gotUser.ID)
	}
	if gotUserID != encodeGraphID("anon", identity.ID) {
		t.Fatalf("userID = %q, want the anon id", gotUserID)
	}
	if gotUser.Name != "匿名005" {
		t.Fatalf("user.name = %q, want 匿名005", gotUser.Name)
	}

	if f.roomFetches != 1 {
		t.Fatalf("room fetches = %d, want 1 (userID と user で2回引いている)", f.roomFetches)
	}
	if f.identityFetches != 1 {
		t.Fatalf("identity fetches = %d, want 1 (userID と user で2回引いている)", f.identityFetches)
	}
	// 授業ルームでは実ユーザーを引く必要がない（引くこと自体が実名漏れの入口になる）。
	if f.userFetches != 0 {
		t.Fatalf("user fetches = %d, want 0 in a course room", f.userFetches)
	}
}

// TestMessageUser_NonCourseRoomKeepsTheRealUser は非授業ルームで実名表示が
// 保たれていること（匿名化の対象を広げていないこと）を確かめる。
func TestMessageUser_NonCourseRoomKeepsTheRealUser(t *testing.T) {
	const (
		roomID   = int64(7)
		authorID = int64(42)
	)
	f := newLoaderFixture(t, fixtureOpts{
		rooms: map[int64]*model.Room{roomID: {ID: roomID, Type: model.RoomTypeCommunity}},
		users: map[int64]*model.User{authorID: realUser(authorID, "山田太郎")},
	})

	mr := &messageResolver{&Resolver{}}
	msg := &gqlmodel.Message{
		ID:     encodeGraphID("message", 100),
		RoomID: encodeGraphID("room", roomID),
		UserID: encodeGraphID("user", authorID),
	}

	gotUserID, err := mr.UserID(f.ctx, msg)
	if err != nil {
		t.Fatalf("UserID: %v", err)
	}
	if gotUserID != msg.UserID {
		t.Fatalf("userID = %q, want the real user id %q", gotUserID, msg.UserID)
	}

	gotUser, err := mr.User(f.ctx, msg)
	if err != nil {
		t.Fatalf("User: %v", err)
	}
	if gotUser == nil || gotUser.Name != "山田太郎" {
		t.Fatalf("user = %+v, want the real user", gotUser)
	}
}
