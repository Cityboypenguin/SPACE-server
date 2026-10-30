package user

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

// deleteUserRepo は退会の段階を覚えておく代役。lifecycles にある人だけが存在する。
type deleteUserRepo struct {
	repository.UserRepository
	lifecycles      map[int64]*model.UserLifecycle
	deactivated     []int64
	purged          []int64
	activityDeleted []int64
	// purgeErr を入れた人の PurgeUser は失敗する。
	purgeErr map[int64]error
}

func (r *deleteUserRepo) DeactivateUser(_ context.Context, id int64, at time.Time) (bool, error) {
	l := r.lifecycles[id]
	if l == nil || l.Status != model.UserStatusActive {
		return false, nil
	}
	l.Status, l.DeactivatedAt = model.UserStatusDeactivated, &at
	r.deactivated = append(r.deactivated, id)
	return true, nil
}

func (r *deleteUserRepo) LockUserLifecycle(_ context.Context, id int64) (*model.UserLifecycle, error) {
	l := r.lifecycles[id]
	if l == nil {
		return nil, nil
	}
	copied := *l
	return &copied, nil
}

func (r *deleteUserRepo) PurgeUser(_ context.Context, id int64, _ time.Time) (bool, error) {
	if err := r.purgeErr[id]; err != nil {
		return false, err
	}
	r.lifecycles[id].Status = model.UserStatusDeleted
	r.purged = append(r.purged, id)
	return true, nil
}

func (r *deleteUserRepo) ListUserIDsToPurge(_ context.Context, before time.Time, limit int) ([]int64, error) {
	var ids []int64
	for id := int64(1); id <= 100 && len(ids) < limit; id++ {
		l := r.lifecycles[id]
		if l != nil && l.Status == model.UserStatusDeactivated && !l.DeactivatedAt.After(before) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (r *deleteUserRepo) DeleteActivityHistory(_ context.Context, id int64) error {
	r.activityDeleted = append(r.activityDeleted, id)
	return nil
}

type deleteUserPostRepo struct{ repository.PostRepository }

func (*deleteUserPostRepo) DeletePostsByUserID(context.Context, int64) error { return nil }
func (*deleteUserPostRepo) RecalculateReplyCountsAffectedByUser(context.Context, int64) error {
	return nil
}

type deleteUserRoomRepo struct {
	repository.RoomRepository
	deletedRooms []int64
}

func (r *deleteUserRoomRepo) DeleteRoom(_ context.Context, roomID int64) (bool, error) {
	r.deletedRooms = append(r.deletedRooms, roomID)
	return true, nil
}

type deleteUserRoomUsers struct {
	repository.RoomUserRepository
	memberships map[int64]string
	roomRoles   map[int64]map[int64]string
	promoted    map[int64]int64 // roomID -> 新しいオーナー
	successors  map[int64]int64 // roomID -> FindOwnerSuccessor が返す人
	demoted     []int64
}

func (r *deleteUserRoomUsers) LockUserCommunityMembershipsForUpdate(context.Context, int64) (map[int64]string, error) {
	return r.memberships, nil
}

func (r *deleteUserRoomUsers) LockRoomMemberRolesForUpdate(_ context.Context, roomID int64) (map[int64]string, error) {
	return r.roomRoles[roomID], nil
}

// FindOwnerSuccessor は、successors に決めてあればその人、無ければ残りのうち
// ID の最も小さい人を返す（参加順や利用状況での選び方は MySQL 側のテストで見る）。
func (r *deleteUserRoomUsers) FindOwnerSuccessor(_ context.Context, roomID, leavingUserID int64) (int64, error) {
	if id, ok := r.successors[roomID]; ok {
		return id, nil
	}
	successor := int64(0)
	for id := range r.roomRoles[roomID] {
		if id != leavingUserID && (successor == 0 || id < successor) {
			successor = id
		}
	}
	return successor, nil
}

func (r *deleteUserRoomUsers) SetRoomUserRole(_ context.Context, roomID, userID int64, role string) error {
	if role != model.RoomUserRoleOwner {
		r.demoted = append(r.demoted, userID)
	}
	if role == model.RoomUserRoleOwner {
		if r.promoted == nil {
			r.promoted = map[int64]int64{}
		}
		r.promoted[roomID] = userID
	}
	return nil
}

type deleteUserMedia struct{ keys map[int64][]string }

func (m *deleteUserMedia) ListStorageKeysByUploader(_ context.Context, userID int64) ([]string, error) {
	return m.keys[userID], nil
}

type deleteUserObjects struct{ discarded []string }

func (o *deleteUserObjects) Discard(_ context.Context, key string) {
	o.discarded = append(o.discarded, key)
}

// deleteUserCommunities は、ルームIDに 1000 を足したものをコミュニティIDとして返す。
type deleteUserCommunities struct{}

func (deleteUserCommunities) GetCommunityIDByRoomID(_ context.Context, roomID int64) (int64, error) {
	return roomID + 1000, nil
}

type deleteUserNotifier struct {
	sent []notificationuc.PublishParams
}

func (n *deleteUserNotifier) PublishBatch(_ context.Context, params []notificationuc.PublishParams) error {
	n.sent = append(n.sent, params...)
	return nil
}

type inlineUserTx struct{}

func (inlineUserTx) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type deletionFixture struct {
	users    *deleteUserRepo
	rooms    *deleteUserRoomRepo
	members  *deleteUserRoomUsers
	media    *deleteUserMedia
	objects  *deleteUserObjects
	notifier *deleteUserNotifier
	now      time.Time
}

func newDeletionFixture() *deletionFixture {
	return &deletionFixture{
		users:    &deleteUserRepo{lifecycles: map[int64]*model.UserLifecycle{}},
		rooms:    &deleteUserRoomRepo{},
		members:  &deleteUserRoomUsers{},
		media:    &deleteUserMedia{keys: map[int64][]string{}},
		objects:  &deleteUserObjects{},
		notifier: &deleteUserNotifier{},
		now:      time.Date(2026, 10, 6, 4, 0, 0, 0, time.UTC),
	}
}

func (f *deletionFixture) deps() AccountDeletionDeps {
	return AccountDeletionDeps{
		Users:       f.users,
		Posts:       &deleteUserPostRepo{},
		Rooms:       f.rooms,
		RoomUsers:   f.members,
		Communities: deleteUserCommunities{},
		Notifier:    f.notifier,
		Media:       f.media,
		Objects:     f.objects,
		TxManager:   inlineUserTx{},
		Now:         func() time.Time { return f.now },
	}
}

func (f *deletionFixture) addUser(id int64, status string, deactivatedAt *time.Time) {
	f.users.lifecycles[id] = &model.UserLifecycle{Status: status, DeactivatedAt: deactivatedAt}
}

func userCtx(userID int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: userID, Role: "user"})
}

func adminCtxFor() context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: 99, Role: "administrator"})
}

// --- 本人の退会 ---------------------------------------------------------------

// 唯一のオーナーでも退会できる。オーナーはその場で残るメンバーに引き継ぎ、
// 本人のオーナーは外さない（取り消したときに元どおり使えるように）。
func TestDeleteMyAccountHandsOverSoleOwnership(t *testing.T) {
	f := newDeletionFixture()
	f.addUser(1, model.UserStatusActive, nil)
	f.members.memberships = map[int64]string{
		10: model.RoomUserRoleOwner, // 唯一のオーナーで他にメンバー → 引き継ぐ
		20: model.RoomUserRoleOwner, // 他にもオーナーが居る → そのまま
		30: model.RoomUserRoleOwner, // 自分しか居ない → 猶予の間は残す
	}
	f.members.roomRoles = map[int64]map[int64]string{
		10: {1: model.RoomUserRoleOwner, 2: model.RoomUserRoleMember, 5: model.RoomUserRoleMember},
		20: {1: model.RoomUserRoleOwner, 4: model.RoomUserRoleOwner, 6: model.RoomUserRoleMember},
		30: {1: model.RoomUserRoleOwner},
	}
	f.members.successors = map[int64]int64{10: 5}

	ok, err := NewDeleteMyAccountUseCase(f.deps()).Execute(userCtx(1))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if f.users.lifecycles[1].Status != model.UserStatusDeactivated {
		t.Fatalf("status = %q, want deactivated", f.users.lifecycles[1].Status)
	}
	if len(f.members.promoted) != 1 || f.members.promoted[10] != 5 {
		t.Fatalf("promoted = %v, want room 10 handed to the successor (user 5)", f.members.promoted)
	}
	if len(f.members.demoted) != 0 || len(f.rooms.deletedRooms) != 0 {
		t.Fatalf("demoted=%v deleted rooms=%v, want neither during the grace period", f.members.demoted, f.rooms.deletedRooms)
	}
	assertHandoverNotified(t, f.notifier.sent, 5, 1010)
}

// assertHandoverNotified は、引き継いだ人へだけ、そのコミュニティを開く通知が1件届いたこと。
func assertHandoverNotified(t *testing.T, sent []notificationuc.PublishParams, successor, communityID int64) {
	t.Helper()
	if len(sent) != 1 {
		t.Fatalf("notifications = %+v, want one for the new owner", sent)
	}
	n := sent[0]
	if n.UserID != successor || n.Type != notificationuc.TypeCommunityRole ||
		n.TargetType == nil || *n.TargetType != notificationuc.TargetCommunity ||
		n.TargetID == nil || *n.TargetID != communityID ||
		n.Message != notificationuc.MessageInheritedCommunityOwner {
		t.Fatalf("notification = %+v, want user %d told they inherited community %d", n, successor, communityID)
	}
}

// 既に退会手続き中なら、もう一度引き継ぎをしない。
func TestDeleteMyAccountTwiceDoesNotHandOverAgain(t *testing.T) {
	f := newDeletionFixture()
	at := f.now.Add(-time.Hour)
	f.addUser(1, model.UserStatusDeactivated, &at)
	f.members.memberships = map[int64]string{10: model.RoomUserRoleOwner}
	f.members.roomRoles = map[int64]map[int64]string{10: {1: model.RoomUserRoleOwner, 2: model.RoomUserRoleMember}}

	if _, err := NewDeleteMyAccountUseCase(f.deps()).Execute(userCtx(1)); err != nil {
		t.Fatal(err)
	}
	if len(f.members.promoted) != 0 || len(f.notifier.sent) != 0 {
		t.Fatalf("promoted=%v notified=%v, want nothing for an already withdrawn user", f.members.promoted, f.notifier.sent)
	}
}

// 本人の退会は退会手続き中にするだけ。猶予の間にログインで戻れるよう、まだ何も消さない。
func TestDeleteMyAccountOnlyDeactivates(t *testing.T) {
	f := newDeletionFixture()
	f.addUser(1, model.UserStatusActive, nil)
	f.members.memberships = map[int64]string{10: model.RoomUserRoleOwner}
	f.members.roomRoles = map[int64]map[int64]string{10: {1: model.RoomUserRoleOwner}}
	f.media.keys[1] = []string{"media/1/a.jpg"}

	ok, err := NewDeleteMyAccountUseCase(f.deps()).Execute(userCtx(1))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got := f.users.lifecycles[1]; got.Status != model.UserStatusDeactivated || !got.DeactivatedAt.Equal(f.now) {
		t.Fatalf("lifecycle = %+v, want deactivated at %v", got, f.now)
	}
	if len(f.users.purged) != 0 || len(f.rooms.deletedRooms) != 0 || len(f.objects.discarded) != 0 || len(f.users.activityDeleted) != 0 {
		t.Fatalf("withdrawal deleted something during the grace period: purged=%v rooms=%v objects=%v activity=%v",
			f.users.purged, f.rooms.deletedRooms, f.objects.discarded, f.users.activityDeleted)
	}
}

// --- 管理者による削除 ---------------------------------------------------------

func TestDeleteUserRequiresAdmin(t *testing.T) {
	f := newDeletionFixture()
	f.addUser(1, model.UserStatusActive, nil)
	if _, err := NewDeleteUserUseCase(f.deps()).Execute(userCtx(1), 1); err == nil {
		t.Fatal("a non-admin must not purge an account")
	}
	if len(f.users.purged) != 0 {
		t.Fatal("rejected purge changed persisted state")
	}
}

func TestDeleteUserPurgesImmediatelyAndSettlesCommunities(t *testing.T) {
	f := newDeletionFixture()
	f.addUser(1, model.UserStatusFrozen, nil)
	f.members.memberships = map[int64]string{
		10: model.RoomUserRoleOwner, // 自分しか居ない → 消す
		20: model.RoomUserRoleOwner, // 唯一のオーナーで他にメンバー → 譲る
		30: model.RoomUserRoleOwner, // 他にもオーナーが居る → そのまま
	}
	f.members.roomRoles = map[int64]map[int64]string{
		10: {1: model.RoomUserRoleOwner},
		20: {1: model.RoomUserRoleOwner, 7: model.RoomUserRoleMember, 3: model.RoomUserRoleMember},
		30: {1: model.RoomUserRoleOwner, 4: model.RoomUserRoleOwner},
	}
	f.media.keys[1] = []string{"media/1/a.jpg", "avatars/1/b.png"}

	ok, err := NewDeleteUserUseCase(f.deps()).Execute(adminCtxFor(), 1)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if len(f.users.purged) != 1 || len(f.users.activityDeleted) != 1 {
		t.Fatalf("purged=%v activity=%v", f.users.purged, f.users.activityDeleted)
	}
	if len(f.rooms.deletedRooms) != 1 || f.rooms.deletedRooms[0] != 10 {
		t.Fatalf("deleted rooms = %v, want [10]", f.rooms.deletedRooms)
	}
	if len(f.members.promoted) != 1 || f.members.promoted[20] != 3 {
		t.Fatalf("promoted = %v, want room 20 handed to the successor (user 3)", f.members.promoted)
	}
	if len(f.objects.discarded) != 2 {
		t.Fatalf("discarded = %v, want both uploaded objects", f.objects.discarded)
	}
	assertHandoverNotified(t, f.notifier.sent, 3, 1020)
}

// 消すのに失敗したら、ストレージ上の実体は消さず、オーナーの引き継ぎも知らせない。
func TestDeleteUserKeepsObjectsWhenPurgeFails(t *testing.T) {
	f := newDeletionFixture()
	f.addUser(1, model.UserStatusActive, nil)
	f.media.keys[1] = []string{"media/1/a.jpg"}
	f.members.memberships = map[int64]string{10: model.RoomUserRoleOwner}
	f.members.roomRoles = map[int64]map[int64]string{10: {1: model.RoomUserRoleOwner, 2: model.RoomUserRoleMember}}
	f.users.purgeErr = map[int64]error{1: errors.New("db is down")}

	if _, err := NewDeleteUserUseCase(f.deps()).Execute(adminCtxFor(), 1); err == nil {
		t.Fatal("want the purge error")
	}
	if len(f.objects.discarded) != 0 {
		t.Fatalf("discarded %v although the purge failed", f.objects.discarded)
	}
	// 引き継ぎもロールバックされるので、知らせない。
	if len(f.notifier.sent) != 0 {
		t.Fatalf("notified %v although the purge failed", f.notifier.sent)
	}
}

// --- 猶予切れ -----------------------------------------------------------------

func TestPurgeExpiredAccountsOnlyPurgesExpiredOnes(t *testing.T) {
	f := newDeletionFixture()
	expiredAt := f.now.Add(-model.AccountDeletionGracePeriod)
	stillInGrace := f.now.Add(-model.AccountDeletionGracePeriod + time.Second)
	f.addUser(1, model.UserStatusDeactivated, &expiredAt)
	f.addUser(2, model.UserStatusDeactivated, &stillInGrace)
	f.addUser(3, model.UserStatusActive, nil)

	n, err := NewPurgeExpiredAccountsUseCase(f.deps()).Execute(context.Background(), f.now)
	if err != nil || n != 1 {
		t.Fatalf("purged=%d err=%v, want 1", n, err)
	}
	if len(f.users.purged) != 1 || f.users.purged[0] != 1 {
		t.Fatalf("purged = %v, want [1]", f.users.purged)
	}
}

// 一覧を取った後に本人がログインして退会を取り消していたら、消さない。
// 消す直前に行をロックして状態を確かめ直すのはこのため。
func TestPurgeExpiredAccountsSkipsUsersWhoCameBack(t *testing.T) {
	f := newDeletionFixture()
	expiredAt := f.now.Add(-model.AccountDeletionGracePeriod - time.Hour)
	f.addUser(1, model.UserStatusDeactivated, &expiredAt)
	repo := &cameBackRepo{deleteUserRepo: f.users}
	deps := f.deps()
	deps.Users = repo

	n, err := NewPurgeExpiredAccountsUseCase(deps).Execute(context.Background(), f.now)
	if err != nil || n != 0 {
		t.Fatalf("purged=%d err=%v, want 0", n, err)
	}
	if len(f.users.purged) != 0 {
		t.Fatalf("purged %v although the user reactivated before the lock", f.users.purged)
	}
}

// cameBackRepo は「一覧には載ったが、ロックした時にはもう active に戻っていた」を作る。
type cameBackRepo struct{ *deleteUserRepo }

func (r *cameBackRepo) LockUserLifecycle(context.Context, int64) (*model.UserLifecycle, error) {
	return &model.UserLifecycle{Status: model.UserStatusActive}, nil
}

// 1人の失敗で他の人を止めない。失敗はまとめて返す。
func TestPurgeExpiredAccountsContinuesPastFailures(t *testing.T) {
	f := newDeletionFixture()
	expiredAt := f.now.Add(-model.AccountDeletionGracePeriod - time.Hour)
	f.addUser(1, model.UserStatusDeactivated, &expiredAt)
	f.addUser(2, model.UserStatusDeactivated, &expiredAt)
	f.users.purgeErr = map[int64]error{1: errors.New("boom")}

	n, err := NewPurgeExpiredAccountsUseCase(f.deps()).Execute(context.Background(), f.now)
	if err == nil {
		t.Fatal("want the failure for user 1 to be reported")
	}
	if n != 1 || len(f.users.purged) != 1 || f.users.purged[0] != 2 {
		t.Fatalf("purged=%d %v, want user 2 only", n, f.users.purged)
	}
}
