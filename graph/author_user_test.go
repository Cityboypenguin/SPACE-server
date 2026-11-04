package graph

import (
	"context"
	"testing"
	"time"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/dataloader"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/vikstrous/dataloadgen"
)

// 投稿者（Message / Question / Answer / Poll の user）の表示の回帰テスト。
//
// 授業内チャットは以前は「匿名NNN」で表示していたが、今はどのルームでも実名で出す。
// 守りたい性質は2つ。
//   - 授業ルームでも実ユーザーがそのまま返ること。
//   - 退会者の投稿で nil を返さないこと（user は非nullなので一覧ごと落ちる）。

// loaderFixture はルームとユーザーのローダーだけを差し込んだコンテキストを作る。
// 使わないローダーは nil のまま（触れば panic するので、経路が変わればすぐ分かる）。
type loaderFixture struct {
	ctx context.Context
}

type fixtureOpts struct {
	rooms   map[int64]*model.Room
	users   map[int64]*model.User
	roomErr error
	userErr error
}

func newLoaderFixture(t *testing.T, opts fixtureOpts) *loaderFixture {
	t.Helper()

	// wait を 0 にして、テストが1件ずつ Load してもバッチ待ちで遅くならないようにする。
	const wait = 0

	roomLoader := dataloadgen.NewLoader(func(_ context.Context, ids []int64) ([]*model.Room, []error) {
		return fixtureBatch(ids, opts.rooms, opts.roomErr)
	}, dataloadgen.WithWait(wait))
	userLoader := dataloadgen.NewLoader(func(_ context.Context, ids []int64) ([]*model.User, []error) {
		return fixtureBatch(ids, opts.users, opts.userErr)
	}, dataloadgen.WithWait(wait))

	return &loaderFixture{ctx: dataloader.WithLoaders(context.Background(), &dataloader.Loaders{
		RoomLoader: roomLoader,
		UserLoader: userLoader,
	})}
}

func fixtureBatch[T any](ids []int64, rows map[int64]*T, err error) ([]*T, []error) {
	out := make([]*T, len(ids))
	errs := make([]error, len(ids))
	for i, id := range ids {
		if err != nil {
			errs[i] = err
			continue
		}
		out[i] = rows[id]
	}
	return out, errs
}

func courseRoom(id int64) *model.Room {
	return &model.Room{ID: id, Name: "授業", Type: model.RoomTypeCourse, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}
}

func realUser(id int64, name string) *model.User {
	return &model.User{ID: id, AccountID: "acct", Name: name, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}
}

func TestMessageUser_CourseRoomShowsTheRealUser(t *testing.T) {
	const authorID = int64(42)
	f := newLoaderFixture(t, fixtureOpts{
		rooms: map[int64]*model.Room{7: courseRoom(7)},
		users: map[int64]*model.User{authorID: realUser(authorID, "山田太郎")},
	})
	r := &messageResolver{&Resolver{}}
	msg := messageIn(7, authorID)

	user, err := r.User(f.ctx, msg)
	if err != nil {
		t.Fatalf("User: %v", err)
	}
	if user.Name != "山田太郎" || user.ID != encodeGraphID("user", authorID) {
		t.Fatalf("user = %+v, want the real author", user)
	}
	userID, err := r.UserID(f.ctx, msg)
	if err != nil {
		t.Fatalf("UserID: %v", err)
	}
	if userID != user.ID {
		t.Fatalf("userID = %q, want it to match user.ID %q", userID, user.ID)
	}
}

// 質問・回答・投票の user も非null。退会者の投稿が1件あるだけで一覧が出なくならないこと。
func TestClassroomAuthors_DeletedUserFallsBackToPlaceholder(t *testing.T) {
	const authorID = int64(42)
	f := newLoaderFixture(t, fixtureOpts{users: map[int64]*model.User{}})
	r := &Resolver{}
	author := &gqlmodel.User{ID: encodeGraphID("user", authorID)}
	roomID := encodeGraphID("room", 7)

	for _, tt := range []struct {
		name    string
		resolve func() (*gqlmodel.User, error)
	}{
		{"question", func() (*gqlmodel.User, error) {
			return (&questionResolver{r}).User(f.ctx, &gqlmodel.Question{RoomID: roomID, User: author})
		}},
		{"answer", func() (*gqlmodel.User, error) {
			return (&answerResolver{r}).User(f.ctx, &gqlmodel.Answer{QuestionID: encodeGraphID("question", 1), User: author})
		}},
		{"poll", func() (*gqlmodel.User, error) {
			return (&pollResolver{r}).User(f.ctx, &gqlmodel.Poll{RoomID: roomID, User: author})
		}},
	} {
		user, err := tt.resolve()
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if user == nil || user.Name != deletedAccountDisplayName {
			t.Fatalf("%s: user = %+v, want the deleted-account placeholder", tt.name, user)
		}
	}
}
