package graph

import (
	"context"
	"net/http"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	chatusecase "github.com/Cityboypenguin/SPACE-server/usecase/chat"
	courseusecase "github.com/Cityboypenguin/SPACE-server/usecase/course"
	pollusecase "github.com/Cityboypenguin/SPACE-server/usecase/poll"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
)

// 「要求されていない派生値を計算しない」という性質は、GraphQL の選択集合が
// 無いと確かめようがない。リゾルバを直接呼ぶテストでは fieldRequested が必ず
// true（判定できないので従来どおり計算する）に倒れるため、本物の実行器を
// 通してクエリを投げる。
//
// 差し込むユースケースは各テストが必要な口だけ。埋めていないフィールドは nil の
// ままなので、経路が変わって別のユースケースを呼び始めれば panic ですぐ分かる。

// newSelectionTestClient は生成済みスキーマに resolver を挿した実行器を作り、
// 全リクエストへ claims を載せるクライアントを返す。claims が nil なら未認証。
func newSelectionTestClient(t *testing.T, resolver *Resolver, claims *auth.Claims) *client.Client {
	t.Helper()

	srv := handler.New(NewExecutableSchema(Config{Resolvers: resolver}))
	srv.AddTransport(transport.POST{})

	withClaims := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if claims != nil {
			r = r.WithContext(auth.WithClaims(r.Context(), claims))
		}
		srv.ServeHTTP(w, r)
	})

	return client.New(withClaims)
}

// adminClaims / userClaims はテスト用の呼び出し元。role は authz.IsAdminRole に合わせる。
func adminClaims(id int64) *auth.Claims { return &auth.Claims{ID: id, Role: "admin"} }
func userClaims(id int64) *auth.Claims  { return &auth.Claims{ID: id, Role: "user"} }

// fieldRequested の判定そのものの回帰テスト。
//
// 誤って false に倒すと「選んだのに値が返らない」という外から見える壊れ方をする
// ので、緩い側（計算してしまう側）へ倒れることを含めて固定しておく。
// ここでは実行器を通した実際のクエリで確かめる（Query.users は total を持つ
// 素直なページ型で、リゾルバは ListUsersUseCase 1つしか呼ばない）。
func TestFieldRequested_SelectionShapes(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		wantTotal bool
	}{
		{
			name:      "total を選ばなければ数えない",
			query:     `{ users { items { ID } } }`,
			wantTotal: false,
		},
		{
			name:      "total を選べば数える",
			query:     `{ users { items { ID } total } }`,
			wantTotal: true,
		},
		{
			name:      "別名でも数える",
			query:     `{ users { count: total } }`,
			wantTotal: true,
		},
		{
			name:      "インラインフラグメントの中でも数える",
			query:     `{ users { ... on UserAccountPage { total } } }`,
			wantTotal: true,
		},
		{
			name:      "名前付きフラグメントの中でも数える",
			query:     `{ users { ...f } } fragment f on UserAccountPage { total }`,
			wantTotal: true,
		},
		{
			name:      "@skip(if: true) で応答に出ないなら数えない",
			query:     `{ users { items { ID } total @skip(if: true) } }`,
			wantTotal: false,
		},
		{
			name:      "@include(if: false) で応答に出ないなら数えない",
			query:     `{ users { items { ID } total @include(if: false) } }`,
			wantTotal: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeListUsersUseCase{}
			r := &Resolver{}
			r.ListUsersUseCase = fake

			c := newSelectionTestClient(t, r, adminClaims(1))
			var resp map[string]any
			c.MustPost(tc.query, &resp)

			if fake.calls != 1 {
				t.Fatalf("ListUsersUseCase calls = %d, want 1", fake.calls)
			}
			if fake.lastQuery.WithTotal != tc.wantTotal {
				t.Errorf("WithTotal = %v, want %v", fake.lastQuery.WithTotal, tc.wantTotal)
			}
		})
	}
}

// fakeListUsersUseCase は Query.users が呼ぶ唯一のユースケース。
// 何回呼ばれたかと、渡された PageQuery（= total を数えるか）を記録する。
type fakeListUsersUseCase struct {
	calls     int
	lastQuery repository.PageQuery
	users     []*model.UserAccount
	total     int
}

func (f *fakeListUsersUseCase) Execute(_ context.Context, q repository.PageQuery) ([]*model.UserAccount, int, error) {
	f.calls++
	f.lastQuery = q
	return f.users, f.total, nil
}

// --- (2) 通知一覧の常時 hydrate -------------------------------------------

type fakeListNotificationsUseCase struct {
	calls         int
	notifications []*model.Notification
	total         int
}

func (f *fakeListNotificationsUseCase) Execute(_ context.Context, _ int64, _ repository.PageQuery) ([]*model.Notification, int, error) {
	f.calls++
	return f.notifications, f.total, nil
}

type fakeGetUsersByIDsUseCase struct {
	calls int
	users []*model.User
}

func (f *fakeGetUsersByIDsUseCase) Execute(_ context.Context, ids []int64) ([]*model.User, error) {
	f.calls++
	out := make([]*model.User, 0, len(ids))
	for _, id := range ids {
		for _, u := range f.users {
			if u.ID == id {
				out = append(out, u)
			}
		}
	}
	return out, nil
}

type fakeGetPostsByIDsUseCase struct {
	calls int
	posts []*model.Post
}

func (f *fakeGetPostsByIDsUseCase) Execute(_ context.Context, _ []int64) ([]*model.Post, error) {
	f.calls++
	return f.posts, nil
}

func int64Ptr(v int64) *int64 { return &v }
func strPtr(v string) *string { return &v }

// 通知一覧は actor / targetPost が選ばれたときだけ、それぞれの一括取得を撃つ。
// 以前は選択に関係なく users の IN 取得と posts の IN 取得が必ず走っていた。
func TestMyNotifications_HydratesOnlyWhatIsSelected(t *testing.T) {
	notifications := []*model.Notification{
		{ID: 1, Type: "reply", Message: "m", ActorID: int64Ptr(9), TargetType: strPtr("post"), TargetID: int64Ptr(5)},
	}

	cases := []struct {
		name          string
		query         string
		wantActorCall int
		wantPostCall  int
	}{
		{
			name:          "本文だけなら actor も post も引かない",
			query:         `{ myNotifications { items { ID message } } }`,
			wantActorCall: 0,
			wantPostCall:  0,
		},
		{
			name:          "actor を選んだら actor だけ引く",
			query:         `{ myNotifications { items { ID actor { ID } } } }`,
			wantActorCall: 1,
			wantPostCall:  0,
		},
		{
			name:          "targetPost を選んだら post だけ引く",
			query:         `{ myNotifications { items { ID targetPost { ID } } } }`,
			wantActorCall: 0,
			wantPostCall:  1,
		},
		{
			name:          "両方選んだら両方引く",
			query:         `{ myNotifications { items { ID actor { ID } targetPost { ID } } } }`,
			wantActorCall: 1,
			wantPostCall:  1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			list := &fakeListNotificationsUseCase{notifications: notifications, total: 1}
			users := &fakeGetUsersByIDsUseCase{users: []*model.User{{ID: 9, AccountID: "actor9", Name: "actor"}}}
			posts := &fakeGetPostsByIDsUseCase{posts: []*model.Post{{ID: 5}}}

			r := &Resolver{}
			r.ListNotificationsUseCase = list
			r.GetUsersByIDsUseCase = users
			r.GetPostsByIDsUseCase = posts

			c := newSelectionTestClient(t, r, userClaims(1))
			var resp map[string]any
			c.MustPost(tc.query, &resp)

			if users.calls != tc.wantActorCall {
				t.Errorf("GetUsersByIDs calls = %d, want %d", users.calls, tc.wantActorCall)
			}
			if posts.calls != tc.wantPostCall {
				t.Errorf("GetPostsByIDs calls = %d, want %d", posts.calls, tc.wantPostCall)
			}
			if list.calls != 1 {
				t.Errorf("ListNotifications calls = %d, want 1", list.calls)
			}
		})
	}
}

// 引くと決めたときは中身まで従来どおり返る（省略の結果 null になっていないこと）。
func TestMyNotifications_StillReturnsActorWhenSelected(t *testing.T) {
	r := &Resolver{}
	r.ListNotificationsUseCase = &fakeListNotificationsUseCase{
		notifications: []*model.Notification{{ID: 1, Type: "reply", Message: "m", ActorID: int64Ptr(9)}},
		total:         1,
	}
	r.GetUsersByIDsUseCase = &fakeGetUsersByIDsUseCase{users: []*model.User{{ID: 9, AccountID: "actor9", Name: "actor"}}}
	r.GetPostsByIDsUseCase = &fakeGetPostsByIDsUseCase{}

	c := newSelectionTestClient(t, r, userClaims(1))
	var resp struct {
		MyNotifications struct {
			Items []struct {
				Actor *struct{ AccountID string }
			}
		}
	}
	c.MustPost(`{ myNotifications { items { actor { accountID } } } }`, &resp)

	if len(resp.MyNotifications.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.MyNotifications.Items))
	}
	if resp.MyNotifications.Items[0].Actor == nil || resp.MyNotifications.Items[0].Actor.AccountID != "actor9" {
		t.Errorf("actor = %+v, want accountID actor9", resp.MyNotifications.Items[0].Actor)
	}
}

// --- (4) コミュニティ一覧の集計 -------------------------------------------

type fakeListAllCommunitiesUseCase struct {
	calls       int
	communities []*model.Community
	total       int
}

func (f *fakeListAllCommunitiesUseCase) Execute(_ context.Context, _ repository.PageQuery) ([]*model.Community, int, error) {
	f.calls++
	return f.communities, f.total, nil
}

type fakeSearchCommunityUseCase struct {
	calls       int
	communities []*model.Community
	total       int
}

func (f *fakeSearchCommunityUseCase) Execute(_ context.Context, _ string, _ repository.PageQuery) ([]*model.Community, int, error) {
	f.calls++
	return f.communities, f.total, nil
}

type fakeCountUsersByRoomIDs struct {
	calls   int
	gotRoom []int64
	counts  map[int64]int
}

func (f *fakeCountUsersByRoomIDs) Execute(_ context.Context, roomIDs []int64) (map[int64]int, error) {
	f.calls++
	f.gotRoom = roomIDs
	return f.counts, nil
}

type fakeListJoinedRoomIDs struct {
	calls  int
	joined map[int64]bool
}

func (f *fakeListJoinedRoomIDs) Execute(_ context.Context, _ int64, _ []int64) (map[int64]bool, error) {
	f.calls++
	return f.joined, nil
}

// コミュニティ一覧は memberCount / isMember が選ばれたときだけ、それぞれ1クエリで
// 集計する。以前は一覧の件数ぶん GetUserIDsByRoomID を呼んでいた（N+1）。
func TestSearchCommunities_AggregatesOncePerRequest(t *testing.T) {
	communities := []*model.Community{
		{ID: 1, RoomID: 11, Name: "a"},
		{ID: 2, RoomID: 12, Name: "b"},
		{ID: 3, RoomID: 13, Name: "c"},
	}

	cases := []struct {
		name          string
		query         string
		wantCountCall int
		wantJoinCall  int
	}{
		{
			name:          "名前だけなら集計しない",
			query:         `{ searchCommunities(name: "x") { items { ID name } } }`,
			wantCountCall: 0,
			wantJoinCall:  0,
		},
		{
			name:          "memberCount を選んだら人数だけ1クエリ",
			query:         `{ searchCommunities(name: "x") { items { ID memberCount } } }`,
			wantCountCall: 1,
			wantJoinCall:  0,
		},
		{
			name:          "isMember を選んだら所属だけ1クエリ",
			query:         `{ searchCommunities(name: "x") { items { ID isMember } } }`,
			wantCountCall: 0,
			wantJoinCall:  1,
		},
		{
			name:          "両方選んでも、件数に関係なく1クエリずつ",
			query:         `{ searchCommunities(name: "x") { items { ID memberCount isMember } } }`,
			wantCountCall: 1,
			wantJoinCall:  1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			counts := &fakeCountUsersByRoomIDs{counts: map[int64]int{11: 5, 12: 6, 13: 7}}
			joined := &fakeListJoinedRoomIDs{joined: map[int64]bool{12: true}}

			r := &Resolver{}
			r.SearchCommunityUseCase = &fakeSearchCommunityUseCase{communities: communities, total: 3}
			r.CountUsersByRoomIDsUseCase = counts
			r.ListJoinedRoomIDsUseCase = joined
			r.StorageRepository = noopStorage{}

			c := newSelectionTestClient(t, r, userClaims(1))
			var resp map[string]any
			c.MustPost(tc.query, &resp)

			if counts.calls != tc.wantCountCall {
				t.Errorf("CountUsersByRoomIDs calls = %d, want %d", counts.calls, tc.wantCountCall)
			}
			if joined.calls != tc.wantJoinCall {
				t.Errorf("ListJoinedRoomIDs calls = %d, want %d", joined.calls, tc.wantJoinCall)
			}
		})
	}
}

// 集計を選んだときの値は従来どおり（まとめて数える形に変えても中身が変わらないこと）。
func TestSearchCommunities_AggregateValuesAreUnchanged(t *testing.T) {
	r := &Resolver{}
	r.SearchCommunityUseCase = &fakeSearchCommunityUseCase{
		communities: []*model.Community{{ID: 1, RoomID: 11}, {ID: 2, RoomID: 12}},
		total:       2,
	}
	r.CountUsersByRoomIDsUseCase = &fakeCountUsersByRoomIDs{counts: map[int64]int{11: 5}}
	r.ListJoinedRoomIDsUseCase = &fakeListJoinedRoomIDs{joined: map[int64]bool{12: true}}
	r.StorageRepository = noopStorage{}

	c := newSelectionTestClient(t, r, userClaims(1))
	var resp struct {
		SearchCommunities struct {
			Items []struct {
				MemberCount int
				IsMember    bool
			}
		}
	}
	c.MustPost(`{ searchCommunities(name: "x") { items { memberCount isMember } } }`, &resp)

	got := resp.SearchCommunities.Items
	if len(got) != 2 {
		t.Fatalf("items = %d, want 2", len(got))
	}
	// 11 は5人・未参加、12 は集計に出てこない（0人）・参加済み。
	if got[0].MemberCount != 5 || got[0].IsMember {
		t.Errorf("item0 = %+v, want memberCount 5 / isMember false", got[0])
	}
	if got[1].MemberCount != 0 || !got[1].IsMember {
		t.Errorf("item1 = %+v, want memberCount 0 / isMember true", got[1])
	}
}

// 管理者の全件一覧は viewer を持たないので isMember を引かない（従来どおり false）。
func TestAdminCommunities_NeverLoadsMembershipFlag(t *testing.T) {
	counts := &fakeCountUsersByRoomIDs{counts: map[int64]int{11: 5}}
	joined := &fakeListJoinedRoomIDs{}

	r := &Resolver{}
	r.ListAllCommunitiesUseCase = &fakeListAllCommunitiesUseCase{
		communities: []*model.Community{{ID: 1, RoomID: 11}},
		total:       1,
	}
	r.CountUsersByRoomIDsUseCase = counts
	r.ListJoinedRoomIDsUseCase = joined
	r.StorageRepository = noopStorage{}

	c := newSelectionTestClient(t, r, adminClaims(1))
	var resp map[string]any
	c.MustPost(`{ communities { items { memberCount isMember } } }`, &resp)

	if joined.calls != 0 {
		t.Errorf("ListJoinedRoomIDs calls = %d, want 0 (viewer が決まらない一覧)", joined.calls)
	}
	if counts.calls != 1 {
		t.Errorf("CountUsersByRoomIDs calls = %d, want 1", counts.calls)
	}
}

// noopStorage は avatarURL の組み立てにしか使わない StorageRepository。
// 署名URLは今回のテストの対象ではないので、キーをそのまま返す。
type noopStorage struct{ repository.StorageRepository }

func (noopStorage) PublicURL(key string) string { return key }

// --- (5) 管理者の授業一覧の履修者数 ---------------------------------------

type fakeListCoursesUseCase struct {
	calls int
	items []*model.Course
	total int
}

func (f *fakeListCoursesUseCase) Execute(_ context.Context, _ courseusecase.ListCoursesParam) ([]*model.Course, int, error) {
	f.calls++
	return f.items, f.total, nil
}

type fakeRegisteredCounts struct {
	calls  int
	gotIDs []int64
	counts map[int64]int
}

func (f *fakeRegisteredCounts) Execute(_ context.Context, courseIDs []int64) (map[int64]int, error) {
	f.calls++
	f.gotIDs = courseIDs
	return f.counts, nil
}

// 管理者の授業一覧は registeredCount が選ばれたときだけ、授業IDの集合で1クエリ。
// 以前は一覧の件数ぶん COUNT を撃っていた。
func TestAdminListCourses_CountsRegistrationsOnceWhenSelected(t *testing.T) {
	courses := []*model.Course{{ID: 1}, {ID: 2}, {ID: 3}}

	t.Run("registeredCount を選ばなければ数えない", func(t *testing.T) {
		counts := &fakeRegisteredCounts{counts: map[int64]int{}}
		r := &Resolver{}
		r.ListCoursesUseCase = &fakeListCoursesUseCase{items: courses, total: 3}
		r.GetCourseRegisteredCountsUseCase = counts

		c := newSelectionTestClient(t, r, adminClaims(1))
		var resp map[string]any
		c.MustPost(`{ adminListCourses { items { ID courseName } } }`, &resp)

		if counts.calls != 0 {
			t.Errorf("GetCourseRegisteredCounts calls = %d, want 0", counts.calls)
		}
	})

	t.Run("選んだら授業の件数に関係なく1クエリ", func(t *testing.T) {
		counts := &fakeRegisteredCounts{counts: map[int64]int{1: 10, 2: 20}}
		r := &Resolver{}
		r.ListCoursesUseCase = &fakeListCoursesUseCase{items: courses, total: 3}
		r.GetCourseRegisteredCountsUseCase = counts

		c := newSelectionTestClient(t, r, adminClaims(1))
		var resp struct {
			AdminListCourses struct {
				Items []struct{ RegisteredCount int }
			}
		}
		c.MustPost(`{ adminListCourses { items { registeredCount } } }`, &resp)

		if counts.calls != 1 {
			t.Fatalf("GetCourseRegisteredCounts calls = %d, want 1", counts.calls)
		}
		if len(counts.gotIDs) != 3 {
			t.Errorf("course ids = %v, want all three in one call", counts.gotIDs)
		}
		got := resp.AdminListCourses.Items
		if len(got) != 3 || got[0].RegisteredCount != 10 || got[1].RegisteredCount != 20 || got[2].RegisteredCount != 0 {
			t.Errorf("registeredCount = %+v, want 10/20/0", got)
		}
	})
}

// --- (1) Room / DM 一覧の常時 hydrate -------------------------------------

type fakeListMyDMRooms struct {
	calls int
	rooms []*model.Room
	total int
}

func (f *fakeListMyDMRooms) Execute(_ context.Context, _ int64, _ repository.PageQuery) ([]*model.Room, int, error) {
	f.calls++
	return f.rooms, f.total, nil
}

type fakeListUsersByRoomIDs struct {
	calls int
	users map[int64][]*model.User
}

func (f *fakeListUsersByRoomIDs) Execute(_ context.Context, _ []int64) (map[int64][]*model.User, error) {
	f.calls++
	return f.users, nil
}

type fakeBlockRelatedUserIDs struct {
	calls   int
	blocked map[int64]bool
}

func (f *fakeBlockRelatedUserIDs) Execute(_ context.Context, _ int64) (map[int64]bool, error) {
	f.calls++
	return f.blocked, nil
}

type fakeRoomReadStatusBatch struct {
	calls  int
	status map[int64]*roomusecase.RoomReadStatus
}

func (f *fakeRoomReadStatusBatch) Execute(_ context.Context, _ []int64, _ int64, _ string) (map[int64]*roomusecase.RoomReadStatus, error) {
	f.calls++
	return f.status, nil
}

type fakeLastMessagesByRoomIDs struct {
	calls    int
	messages map[int64]*model.Message
}

func (f *fakeLastMessagesByRoomIDs) Execute(_ context.Context, _ []int64) (map[int64]*model.Message, error) {
	f.calls++
	return f.messages, nil
}

type dmRoomsFixture struct {
	resolver *Resolver
	members  *fakeListUsersByRoomIDs
	blocks   *fakeBlockRelatedUserIDs
	reads    *fakeRoomReadStatusBatch
	last     *fakeLastMessagesByRoomIDs
}

func newDMRoomsFixture() *dmRoomsFixture {
	f := &dmRoomsFixture{
		members: &fakeListUsersByRoomIDs{users: map[int64][]*model.User{
			7: {{ID: 1, AccountID: "me"}, {ID: 2, AccountID: "partner"}},
		}},
		blocks: &fakeBlockRelatedUserIDs{blocked: map[int64]bool{2: true}},
		reads: &fakeRoomReadStatusBatch{status: map[int64]*roomusecase.RoomReadStatus{
			7: {UnreadCount: 3},
		}},
		last: &fakeLastMessagesByRoomIDs{messages: map[int64]*model.Message{
			7: {ID: 100, Content: "hello"},
		}},
	}
	r := &Resolver{}
	r.ListMyDMRoomsUseCase = &fakeListMyDMRooms{
		rooms: []*model.Room{{ID: 7, Name: "dm", Type: model.RoomTypeDM}},
		total: 1,
	}
	r.ListUsersByRoomIDsUseCase = f.members
	r.GetBlockRelatedUserIDsUseCase = f.blocks
	r.GetRoomReadStatusBatchUseCase = f.reads
	r.GetLastMessagesByRoomIDsUseCase = f.last
	f.resolver = r
	return f
}

// DM 一覧は、メンバー・ブロック・既読・最新メッセージを「選ばれたときだけ」引く。
// 以前は選択に関係なく4つとも必ず走っていた。
func TestMyDMRooms_HydratesOnlyWhatIsSelected(t *testing.T) {
	cases := []struct {
		name                                       string
		query                                      string
		wantMembers, wantBlocks, wantReads, wantLM int
	}{
		{
			name:  "名前と種別だけなら付帯情報は引かない",
			query: `{ myDMRooms { items { ID name type } } }`,
		},
		{
			name:        "user はメンバーだけ",
			query:       `{ myDMRooms { items { user { ID } } } }`,
			wantMembers: 1,
		},
		{
			name:        "isMessagingDisabled はメンバー（相手の特定）とブロック",
			query:       `{ myDMRooms { items { isMessagingDisabled } } }`,
			wantMembers: 1,
			wantBlocks:  1,
		},
		{
			name:      "unreadCount は既読だけ",
			query:     `{ myDMRooms { items { unreadCount } } }`,
			wantReads: 1,
		},
		{
			name:      "lastReadMessageID も同じ1回の取得から埋まる",
			query:     `{ myDMRooms { items { lastReadMessageID } } }`,
			wantReads: 1,
		},
		{
			name:   "content（プレビュー）は最新メッセージだけ",
			query:  `{ myDMRooms { items { content } } }`,
			wantLM: 1,
		},
		{
			name:        "全部選べば全部引く（それでも一覧あたり1回ずつ）",
			query:       `{ myDMRooms { items { user { ID } isMessagingDisabled unreadCount content } } }`,
			wantMembers: 1,
			wantBlocks:  1,
			wantReads:   1,
			wantLM:      1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDMRoomsFixture()
			c := newSelectionTestClient(t, f.resolver, userClaims(1))
			var resp map[string]any
			c.MustPost(tc.query, &resp)

			if f.members.calls != tc.wantMembers {
				t.Errorf("ListUsersByRoomIDs calls = %d, want %d", f.members.calls, tc.wantMembers)
			}
			if f.blocks.calls != tc.wantBlocks {
				t.Errorf("GetBlockRelatedUserIDs calls = %d, want %d", f.blocks.calls, tc.wantBlocks)
			}
			if f.reads.calls != tc.wantReads {
				t.Errorf("GetRoomReadStatusBatch calls = %d, want %d", f.reads.calls, tc.wantReads)
			}
			if f.last.calls != tc.wantLM {
				t.Errorf("GetLastMessagesByRoomIDs calls = %d, want %d", f.last.calls, tc.wantLM)
			}
		})
	}
}

// 選んだときの値は従来どおり。省略の結果ゼロ値が出ていないことを確かめる。
func TestMyDMRooms_ValuesAreUnchangedWhenSelected(t *testing.T) {
	f := newDMRoomsFixture()
	c := newSelectionTestClient(t, f.resolver, userClaims(1))

	var resp struct {
		MyDMRooms struct {
			Items []struct {
				User                []struct{ AccountID string }
				IsMessagingDisabled bool
				UnreadCount         int
				Content             *string
			}
		}
	}
	c.MustPost(`{ myDMRooms { items { user { accountID } isMessagingDisabled unreadCount content } } }`, &resp)

	items := resp.MyDMRooms.Items
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if len(items[0].User) != 2 {
		t.Errorf("user = %d, want 2", len(items[0].User))
	}
	if !items[0].IsMessagingDisabled {
		t.Error("isMessagingDisabled = false, want true (相手をブロック中)")
	}
	if items[0].UnreadCount != 3 {
		t.Errorf("unreadCount = %d, want 3", items[0].UnreadCount)
	}
	if items[0].Content == nil || *items[0].Content != "hello" {
		t.Errorf("content = %v, want hello", items[0].Content)
	}
}

// --- (1) room クエリ: 権限判定の順序と、付帯情報の遅延 ---------------------

type fakeGetRoom struct{ rooms map[int64]*model.Room }

func (f *fakeGetRoom) Execute(_ context.Context, id int64) (*model.Room, error) {
	return f.rooms[id], nil
}

type fakeGetRoomMemberIDs struct{ members map[int64][]int64 }

func (f *fakeGetRoomMemberIDs) Execute(_ context.Context, roomID int64) ([]int64, error) {
	return f.members[roomID], nil
}

// fakeIsRoomMember は在籍の有無だけを返す口。同じ members を見るので、
// fakeGetRoomMemberIDs と答えが食い違うことはない。
type fakeIsRoomMember struct{ members map[int64][]int64 }

func (f *fakeIsRoomMember) Execute(_ context.Context, roomID, userID int64) (bool, error) {
	for _, id := range f.members[roomID] {
		if id == userID {
			return true, nil
		}
	}
	return false, nil
}

type fakeCheckRoomWritable struct{}

func (fakeCheckRoomWritable) Execute(context.Context, int64) error { return nil }

type fakeCheckBlockRelation struct {
	calls   int
	blocked bool
}

func (f *fakeCheckBlockRelation) Execute(context.Context, int64, int64) (bool, error) {
	f.calls++
	return f.blocked, nil
}

type fakeReadStatusOfRoom struct {
	calls  int
	status *roomusecase.RoomReadStatus
}

func (f *fakeReadStatusOfRoom) MarkAsRead(context.Context, int64, *int64) error { return nil }
func (f *fakeReadStatusOfRoom) GetReadStatus(context.Context, int64) (*roomusecase.RoomReadStatus, error) {
	f.calls++
	return f.status, nil
}

func (f *fakeReadStatusOfRoom) ReadStatusOfAuthorizedRoom(context.Context, *model.Room) (*roomusecase.RoomReadStatus, error) {
	f.calls++
	return f.status, nil
}

type roomQueryFixture struct {
	resolver *Resolver
	members  *fakeListUsersByRoomIDs
	block    *fakeCheckBlockRelation
	reads    *fakeReadStatusOfRoom
}

// newRoomQueryFixture は room クエリだけを通せる最小構成。
// 閲覧権限は本物の AccessPolicy（chatusecase）を通す。権限判定を fake で
// 置き換えてしまうと、この項目で一番確かめたい「付帯情報を遅らせても権限判定が
// 先に走る」が確かめられなくなるため。
func newRoomQueryFixture(memberIDs []int64) *roomQueryFixture {
	room := &model.Room{ID: 7, Name: "dm", Type: model.RoomTypeDM}

	access := chatusecase.NewAccessPolicy(chatusecase.AccessPolicyDeps{
		GetRoom:            &fakeGetRoom{rooms: map[int64]*model.Room{7: room}},
		GetRoomMemberIDs:   &fakeGetRoomMemberIDs{members: map[int64][]int64{7: memberIDs}},
		IsRoomMember:       &fakeIsRoomMember{members: map[int64][]int64{7: memberIDs}},
		CheckRoomWritable:  fakeCheckRoomWritable{},
		CheckBlockRelation: &fakeCheckBlockRelation{},
	})

	f := &roomQueryFixture{
		members: &fakeListUsersByRoomIDs{users: map[int64][]*model.User{
			7: {{ID: 1, AccountID: "me"}, {ID: 2, AccountID: "partner"}},
		}},
		block: &fakeCheckBlockRelation{blocked: true},
		reads: &fakeReadStatusOfRoom{status: &roomusecase.RoomReadStatus{UnreadCount: 4}},
	}

	r := &Resolver{}
	r.ChatAccess = access
	r.ChatReads = f.reads
	r.ListUsersByRoomIDsUseCase = f.members
	r.CheckBlockRelationUseCase = f.block
	f.resolver = r
	return f
}

// room クエリも DM 一覧と同じで、メンバー・ブロック・既読を選ばれたときだけ引く。
func TestRoomQuery_HydratesOnlyWhatIsSelected(t *testing.T) {
	roomGraphID := encodeGraphID("room", 7)

	cases := []struct {
		name                              string
		selection                         string
		wantMembers, wantBlock, wantReads int
	}{
		{name: "名前だけなら何も引かない", selection: "name type"},
		{name: "user はメンバーだけ", selection: "user { ID }", wantMembers: 1},
		{name: "isMessagingDisabled はメンバーとブロック", selection: "isMessagingDisabled", wantMembers: 1, wantBlock: 1},
		{name: "unreadCount は既読だけ", selection: "unreadCount", wantReads: 1},
		{name: "partnerLastReadAt も同じ取得から埋まる", selection: "partnerLastReadAt", wantReads: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRoomQueryFixture([]int64{1, 2})
			c := newSelectionTestClient(t, f.resolver, userClaims(1))
			var resp map[string]any
			c.MustPost(`query($id: ID!){ room(id: $id) { `+tc.selection+` } }`, &resp, client.Var("id", roomGraphID))

			if f.members.calls != tc.wantMembers {
				t.Errorf("ListUsersByRoomIDs calls = %d, want %d", f.members.calls, tc.wantMembers)
			}
			if f.block.calls != tc.wantBlock {
				t.Errorf("CheckBlockRelation calls = %d, want %d", f.block.calls, tc.wantBlock)
			}
			if f.reads.calls != tc.wantReads {
				t.Errorf("ReadStatusOfAuthorizedRoom calls = %d, want %d", f.reads.calls, tc.wantReads)
			}
		})
	}
}

// 付帯情報を遅らせても、閲覧権限の判定は必ず先に走る。
// 非参加者には従来どおりエラーを返し、メンバー・ブロック・既読は一切引かない
// （権限判定を通っていないルームの情報が、遅延のせいで先に漏れないこと）。
func TestRoomQuery_DeniesBeforeAnyHydration(t *testing.T) {
	f := newRoomQueryFixture([]int64{2, 3}) // 呼び出し元 (1) は参加していない

	c := newSelectionTestClient(t, f.resolver, userClaims(1))
	var resp map[string]any
	err := c.Post(
		`query($id: ID!){ room(id: $id) { user { ID } isMessagingDisabled unreadCount } }`,
		&resp, client.Var("id", encodeGraphID("room", 7)),
	)
	if err == nil {
		t.Fatal("非参加者の room クエリが成功してしまった")
	}
	if f.members.calls != 0 || f.block.calls != 0 || f.reads.calls != 0 {
		t.Errorf("権限判定より先に付帯情報を引いている: members=%d block=%d reads=%d",
			f.members.calls, f.block.calls, f.reads.calls)
	}
}

// --- (3) ページ型の total ---------------------------------------------------

type fakeListAdministrators struct {
	calls     int
	lastQuery repository.PageQuery
}

func (f *fakeListAdministrators) Execute(_ context.Context, q repository.PageQuery) ([]*model.Administrator, int, error) {
	f.calls++
	f.lastQuery = q
	return nil, 0, nil
}

type fakeListPosts struct {
	calls     int
	lastQuery repository.PageQuery
}

func (f *fakeListPosts) Execute(_ context.Context, q repository.PageQuery) ([]*model.Post, int, error) {
	f.calls++
	f.lastQuery = q
	return nil, 0, nil
}

type fakeListPollsUseCase struct {
	calls     int
	lastQuery pollusecase.ListPollsQuery
}

func (f *fakeListPollsUseCase) Execute(_ context.Context, _ int64, q pollusecase.ListPollsQuery) ([]*model.Poll, int, int, error) {
	f.calls++
	f.lastQuery = q
	return nil, 0, 0, nil
}

// total を数えるかどうかの判定は resolvePageQuery 1本に寄せてあるので、
// ページ型が増えても同じ形で効く。代表を数種類だけ固定しておく。
func TestPageTotalsAreOnlyCountedWhenSelected(t *testing.T) {
	t.Run("AdministratorPage", func(t *testing.T) {
		for _, tc := range []struct {
			query string
			want  bool
		}{
			{`{ administrators { items { ID } } }`, false},
			{`{ administrators { total } }`, true},
		} {
			fake := &fakeListAdministrators{}
			r := &Resolver{}
			r.ListAdministratorsUseCase = fake
			c := newSelectionTestClient(t, r, adminClaims(1))
			var resp map[string]any
			c.MustPost(tc.query, &resp)
			if fake.lastQuery.WithTotal != tc.want {
				t.Errorf("%s: WithTotal = %v, want %v", tc.query, fake.lastQuery.WithTotal, tc.want)
			}
		}
	})

	t.Run("PostPage", func(t *testing.T) {
		for _, tc := range []struct {
			query string
			want  bool
		}{
			{`{ posts { items { ID } } }`, false},
			{`{ posts { items { ID } total } }`, true},
		} {
			fake := &fakeListPosts{}
			r := &Resolver{}
			r.ListPostsUseCase = fake
			c := newSelectionTestClient(t, r, adminClaims(1))
			var resp map[string]any
			c.MustPost(tc.query, &resp)
			if fake.lastQuery.WithTotal != tc.want {
				t.Errorf("%s: WithTotal = %v, want %v", tc.query, fake.lastQuery.WithTotal, tc.want)
			}
		}
	})

	// PollPage だけは total と unvotedTotal の2つ。片方だけ選んでも
	// もう片方は数えない。
	t.Run("PollPage は total と unvotedTotal を別々に判定する", func(t *testing.T) {
		roomID := encodeGraphID("room", 7)
		for _, tc := range []struct {
			selection            string
			wantTotal, wantUnvot bool
		}{
			{"items { ID }", false, false},
			{"total", true, false},
			{"unvotedTotal", false, true},
			{"total unvotedTotal", true, true},
		} {
			fake := &fakeListPollsUseCase{}
			f := newRoomQueryFixture([]int64{1, 2})
			f.resolver.ListPollsUseCase = fake

			c := newSelectionTestClient(t, f.resolver, userClaims(1))
			var resp map[string]any
			c.MustPost(`query($id: ID!){ polls(roomID: $id) { `+tc.selection+` } }`, &resp, client.Var("id", roomID))

			if fake.lastQuery.Page.WithTotal != tc.wantTotal {
				t.Errorf("%s: WithTotal = %v, want %v", tc.selection, fake.lastQuery.Page.WithTotal, tc.wantTotal)
			}
			if fake.lastQuery.WithUnvotedTotal != tc.wantUnvot {
				t.Errorf("%s: WithUnvotedTotal = %v, want %v", tc.selection, fake.lastQuery.WithUnvotedTotal, tc.wantUnvot)
			}
		}
	})
}
