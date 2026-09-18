package community

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// countingRoomUserRepo は「何回リポジトリを呼んだか」を数えるだけの実装。
// 未使用のメソッドはインターフェース埋め込みに任せる（呼ばれたら nil panic で気づける）。
type countingRoomUserRepo struct {
	repository.RoomUserRepository

	members []*model.RoomMember

	roleCalls   int
	removeCalls int
	roleSets    map[string][]int64
	removed     []int64
}

func (r *countingRoomUserRepo) ListRoomMembersWithRoles(context.Context, int64) ([]*model.RoomMember, error) {
	return r.members, nil
}

func (r *countingRoomUserRepo) SetRoomUserRoles(_ context.Context, _ int64, userIDs []int64, role string) error {
	r.roleCalls++
	if r.roleSets == nil {
		r.roleSets = map[string][]int64{}
	}
	r.roleSets[role] = append(r.roleSets[role], userIDs...)
	return nil
}

func (r *countingRoomUserRepo) RemoveUsersFromRoom(_ context.Context, _ int64, userIDs []int64) error {
	r.removeCalls++
	r.removed = append(r.removed, userIDs...)
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

func memberWithRole(id int64, role string) *model.RoomMember {
	return &model.RoomMember{User: &model.User{ID: id}, Role: role}
}

// メンバー編集は人数ぶんではなく「操作の種類ぶん」しか DB を叩かない。
// 以前は updates を1件ずつ回していたので、10人チェックすれば10往復していた。
func TestUpdateCommunityMembers_GroupsWritesByAction(t *testing.T) {
	members := []*model.RoomMember{
		memberWithRole(1, model.RoomUserRoleOwner),
		memberWithRole(2, model.RoomUserRoleMember),
		memberWithRole(3, model.RoomUserRoleMember),
		memberWithRole(4, model.RoomUserRoleOwner),
		memberWithRole(5, model.RoomUserRoleOwner),
		memberWithRole(6, model.RoomUserRoleMember),
		memberWithRole(7, model.RoomUserRoleMember),
	}
	roomUsers := &countingRoomUserRepo{members: members}
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
	roomUsers := &countingRoomUserRepo{members: []*model.RoomMember{
		memberWithRole(1, model.RoomUserRoleOwner),
		memberWithRole(2, model.RoomUserRoleMember),
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
	roomUsers := &countingRoomUserRepo{members: []*model.RoomMember{
		memberWithRole(1, model.RoomUserRoleOwner),
		memberWithRole(2, model.RoomUserRoleMember),
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
