package community

import (
	"context"
	"sync"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// countingRoomUserRepo は「何回リポジトリを呼んだか」を数えるだけの実装。
// 未使用のメソッドはインターフェース埋め込みに任せる（呼ばれたら nil panic で気づける）。
type countingRoomUserRepo struct {
	repository.RoomUserRepository

	roles map[int64]string

	roleCalls   int
	removeCalls int
	roleSets    map[string][]int64
	removed     []int64
}

func (r *countingRoomUserRepo) LockRoomMemberRolesForUpdate(context.Context, int64) (map[int64]string, error) {
	roles := make(map[int64]string, len(r.roles))
	for id, role := range r.roles {
		roles[id] = role
	}
	return roles, nil
}

func (r *countingRoomUserRepo) SetRoomUserRoles(_ context.Context, _ int64, userIDs []int64, role string) error {
	r.roleCalls++
	if r.roleSets == nil {
		r.roleSets = map[string][]int64{}
	}
	r.roleSets[role] = append(r.roleSets[role], userIDs...)
	for _, id := range userIDs {
		r.roles[id] = role
	}
	return nil
}

func (r *countingRoomUserRepo) RemoveUsersFromRoom(_ context.Context, _ int64, userIDs []int64) error {
	r.removeCalls++
	r.removed = append(r.removed, userIDs...)
	for _, id := range userIDs {
		delete(r.roles, id)
	}
	return nil
}

type stubCommunityRepo struct {
	repository.CommunityRepository
	community *model.Community
}

func (r *stubCommunityRepo) GetCommunityByID(context.Context, int64) (*model.Community, error) {
	return r.community, nil
}

type inlineTxManager struct{}

func (inlineTxManager) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type serialTxManager struct {
	mu sync.Mutex
}

func (m *serialTxManager) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fn(ctx)
}

// メンバー編集は人数ぶんではなく「操作の種類ぶん」しか DB を叩かない。
// 以前は updates を1件ずつ回していたので、10人チェックすれば10往復していた。
func TestUpdateCommunityMembers_GroupsWritesByAction(t *testing.T) {
	roles := map[int64]string{
		1: model.RoomUserRoleOwner,
		2: model.RoomUserRoleMember,
		3: model.RoomUserRoleMember,
		4: model.RoomUserRoleOwner,
		5: model.RoomUserRoleOwner,
		6: model.RoomUserRoleMember,
		7: model.RoomUserRoleMember,
	}
	roomUsers := &countingRoomUserRepo{roles: roles}
	uc := NewUpdateCommunityMembersUseCase(
		&stubCommunityRepo{community: &model.Community{ID: 10, RoomID: 20}},
		roomUsers,
		inlineTxManager{},
	)

	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "admin"})
	err := uc.Execute(ctx, 10, []MemberUpdate{
		{UserID: 2, Action: MemberActionPromote},
		{UserID: 3, Action: MemberActionPromote},
		{UserID: 4, Action: MemberActionDemote},
		{UserID: 5, Action: MemberActionDemote},
		{UserID: 6, Action: MemberActionKick},
		{UserID: 7, Action: MemberActionKick},
	})
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}

	// 昇格1本 + 降格1本 = 2本。6人ぶんでも2本。
	if roomUsers.roleCalls != 2 {
		t.Fatalf("expected 2 role updates (one per target role), got %d", roomUsers.roleCalls)
	}
	if roomUsers.removeCalls != 1 {
		t.Fatalf("expected 1 removal statement, got %d", roomUsers.removeCalls)
	}

	assertIDs(t, "promoted", roomUsers.roleSets[model.RoomUserRoleOwner], []int64{2, 3})
	assertIDs(t, "demoted", roomUsers.roleSets[model.RoomUserRoleMember], []int64{4, 5})
	assertIDs(t, "removed", roomUsers.removed, []int64{6, 7})
}

// 操作が1種類しか無いときは、その1本だけが出る（要らない往復を作らない）。
func TestUpdateCommunityMembers_SkipsUnusedActions(t *testing.T) {
	roomUsers := &countingRoomUserRepo{roles: map[int64]string{
		1: model.RoomUserRoleOwner,
		2: model.RoomUserRoleMember,
	}}
	uc := NewUpdateCommunityMembersUseCase(
		&stubCommunityRepo{community: &model.Community{ID: 10, RoomID: 20}},
		roomUsers,
		inlineTxManager{},
	)

	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "admin"})
	if err := uc.Execute(ctx, 10, []MemberUpdate{{UserID: 2, Action: MemberActionKick}}); err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}
	if roomUsers.roleCalls != 0 {
		t.Fatalf("expected no role updates, got %d", roomUsers.roleCalls)
	}
	if roomUsers.removeCalls != 1 {
		t.Fatalf("expected 1 removal statement, got %d", roomUsers.removeCalls)
	}
}

// 最後のオーナーが居なくなる編集は、DB へ1本も出さずに弾かれる（従来どおり）。
func TestUpdateCommunityMembers_StillRejectsLosingTheLastOwner(t *testing.T) {
	roomUsers := &countingRoomUserRepo{roles: map[int64]string{
		1: model.RoomUserRoleOwner,
		2: model.RoomUserRoleMember,
	}}
	uc := NewUpdateCommunityMembersUseCase(
		&stubCommunityRepo{community: &model.Community{ID: 10, RoomID: 20}},
		roomUsers,
		inlineTxManager{},
	)

	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "admin"})
	if err := uc.Execute(ctx, 10, []MemberUpdate{{UserID: 1, Action: MemberActionDemote}}); err == nil {
		t.Fatal("expected the update to be rejected")
	}
	if roomUsers.roleCalls != 0 || roomUsers.removeCalls != 0 {
		t.Fatalf("a rejected update must not write: %d role updates, %d removals", roomUsers.roleCalls, roomUsers.removeCalls)
	}
}

func TestUpdateCommunityMembers_ConcurrentDemotionsKeepAnOwner(t *testing.T) {
	roomUsers := &countingRoomUserRepo{roles: map[int64]string{
		1: model.RoomUserRoleOwner,
		2: model.RoomUserRoleOwner,
	}}
	txManager := &serialTxManager{}
	uc := NewUpdateCommunityMembersUseCase(
		&stubCommunityRepo{community: &model.Community{ID: 10, RoomID: 20}},
		roomUsers,
		txManager,
	)
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 99, Role: "admin"})

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, userID := range []int64{1, 2} {
		go func(id int64) {
			<-start
			results <- uc.Execute(ctx, 10, []MemberUpdate{{UserID: id, Action: MemberActionDemote}})
		}(userID)
	}
	close(start)

	successes := 0
	for range 2 {
		if err := <-results; err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful demotions = %d, want 1", successes)
	}

	owners := 0
	for _, role := range roomUsers.roles {
		if role == model.RoomUserRoleOwner {
			owners++
		}
	}
	if owners != 1 {
		t.Fatalf("owners after concurrent demotions = %d, want 1", owners)
	}
}

func assertIDs(t *testing.T, what string, got, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
	seen := map[int64]bool{}
	for _, id := range got {
		seen[id] = true
	}
	for _, id := range want {
		if !seen[id] {
			t.Fatalf("%s: %d is missing from %v", what, id, got)
		}
	}
}
