package user

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

// 退会の入口は3つある。
//
//   - 本人の退会（DeleteMyAccountUseCase）: 退会手続き中にするだけ。猶予の間に
//     ログインすれば取り消せるので、まだ何も消さない。
//   - 管理者による削除（DeleteUserUseCase）: 猶予を置かずにその場で個人情報を消す。
//     本人がログインして取り消せてしまうと削除の意味が無いため。
//   - 猶予切れ（PurgeExpiredAccountsUseCase）: 日次の処理が、猶予の切れた
//     退会手続き中の人の個人情報を消す。
//
// 個人情報を消す手順（purge）は後ろの2つで共通なので accountDeletion に1つだけ置く。
// 消すのは user_accounts の行で、本人の持ち物（プロフィール・投稿・フォロー・添付
// など）は外部キーの CASCADE で一緒に消え、会話（メッセージ・質問・回答・投票）は
// 投稿者を「削除されたアカウント」にして残る（db/migrations/076 参照）。

// uploadedMediaLister は、個人情報を消す前に添付の保存先を控えるための口。
// media 行は CASCADE で消えるが、ストレージ上の実体は消えないので先に引いておく。
type uploadedMediaLister interface {
	ListStorageKeysByUploader(ctx context.Context, userID int64) ([]string, error)
}

// objectDiscarder はストレージ上の実体を消す口。キーから公開・非公開の置き場を
// 選ぶのは実装（internal/upload.Acceptor）の仕事で、ここは知らない。
// 消せなくても退会は成立している（DB からはもう参照されない）ので、結果は返さない。
type objectDiscarder interface {
	Discard(ctx context.Context, objectKey string)
}

// communityIDLookup は、引き継ぎの通知の遷移先（コミュニティ）を引く口。
type communityIDLookup interface {
	GetCommunityIDByRoomID(ctx context.Context, roomID int64) (int64, error)
}

// notificationBatchPublisher は、オーナーを引き継いだ人へ知らせる口。
type notificationBatchPublisher interface {
	PublishBatch(ctx context.Context, params []notificationuc.PublishParams) error
}

// accountDeletion は3つの入口が共有する依存と手順。
type accountDeletion struct {
	users       userDeletionRepository
	posts       repository.PostWriter
	rooms       repository.RoomRepository
	roomUsers   repository.RoomRoleRepository
	communities communityIDLookup
	notifier    notificationBatchPublisher
	media       uploadedMediaLister
	objects     objectDiscarder
	txManager   repository.TxManager
	now         func() time.Time
}

// AccountDeletionDeps は退会の3つのユースケースを組み立てる材料。
type AccountDeletionDeps struct {
	Users     userDeletionRepository
	Posts     repository.PostWriter
	Rooms     repository.RoomRepository
	RoomUsers repository.RoomRoleRepository
	// Communities と Notifier は、オーナーを引き継いだ人へ知らせるために使う。
	Communities communityIDLookup
	Notifier    notificationBatchPublisher
	Media       uploadedMediaLister
	Objects     objectDiscarder
	TxManager   repository.TxManager
	// Now は現在時刻。nil なら time.Now（テストから差し替えるための口）。
	Now func() time.Time
}

func newAccountDeletion(deps AccountDeletionDeps) *accountDeletion {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &accountDeletion{
		users:       deps.Users,
		posts:       deps.Posts,
		rooms:       deps.Rooms,
		roomUsers:   deps.RoomUsers,
		communities: deps.Communities,
		notifier:    deps.Notifier,
		media:       deps.Media,
		objects:     deps.Objects,
		txManager:   deps.TxManager,
		now:         now,
	}
}

// ownedCommunities は userID がオーナーのコミュニティを、メンバーの役割ごと
// ロックして返す。ルームIDの昇順に並べるのは、ロックの順番を揃えてデッドロックを避けるため。
func (d *accountDeletion) ownedCommunities(ctx context.Context, userID int64) ([]int64, map[int64]map[int64]string, error) {
	memberships, err := d.roomUsers.LockUserCommunityMembershipsForUpdate(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	owned := make([]int64, 0)
	for roomID, role := range memberships {
		if role == model.RoomUserRoleOwner {
			owned = append(owned, roomID)
		}
	}
	sort.Slice(owned, func(i, j int) bool { return owned[i] < owned[j] })

	roles := make(map[int64]map[int64]string, len(owned))
	for _, roomID := range owned {
		r, err := d.roomUsers.LockRoomMemberRolesForUpdate(ctx, roomID)
		if err != nil {
			return nil, nil, err
		}
		roles[roomID] = r
	}
	return owned, roles, nil
}

// isSoleOwnerAmongOthers は、userID が唯一のオーナーで、他にメンバーが居るか。
func isSoleOwnerAmongOthers(userID int64, roles map[int64]string) bool {
	if roles[userID] != model.RoomUserRoleOwner || len(roles) <= 1 {
		return false
	}
	for id, role := range roles {
		if id != userID && role == model.RoomUserRoleOwner {
			return false
		}
	}
	return true
}

// ownerHandover は、オーナーを引き継いだ記録。コミットした後に本人へ知らせる。
type ownerHandover struct {
	roomID    int64
	successor int64
}

// handOverOwnerships は、userID が唯一のオーナーで他にメンバーが居るコミュニティの
// オーナーを、残るメンバーに引き継ぐ（誰に渡すかは FindOwnerSuccessor）。
// userID 自身のオーナーは外さない。退会を取り消したときに元どおり使えるようにするため。
// 個人情報を消すときは room_users の行ごと消えるので、外す必要も無い。
func (d *accountDeletion) handOverOwnerships(ctx context.Context, userID int64, owned []int64, roles map[int64]map[int64]string) ([]ownerHandover, error) {
	var handovers []ownerHandover
	for _, roomID := range owned {
		if !isSoleOwnerAmongOthers(userID, roles[roomID]) {
			continue
		}
		successor, err := d.roomUsers.FindOwnerSuccessor(ctx, roomID, userID)
		if err != nil {
			return nil, err
		}
		if successor == 0 {
			return nil, fmt.Errorf("community %d: no member to hand ownership to", roomID)
		}
		if err := d.roomUsers.SetRoomUserRole(ctx, roomID, successor, model.RoomUserRoleOwner); err != nil {
			return nil, err
		}
		handovers = append(handovers, ownerHandover{roomID: roomID, successor: successor})
	}
	return handovers, nil
}

// notifyNewOwners は、オーナーを引き継いだ人へ知らせる。コミットした後に呼ぶ
// （ロールバックした引き継ぎを知らせないため）。退会はもう成立しているので、
// 知らせられなくても失敗にはせず記録だけ残す。
func (d *accountDeletion) notifyNewOwners(ctx context.Context, handovers []ownerHandover) {
	if len(handovers) == 0 {
		return
	}
	targetType := notificationuc.TargetCommunity
	params := make([]notificationuc.PublishParams, 0, len(handovers))
	for _, h := range handovers {
		communityID, err := d.communities.GetCommunityIDByRoomID(ctx, h.roomID)
		if err != nil || communityID == 0 {
			logger.Log.Error().Err(err).Int64("room_id", h.roomID).
				Msg("failed to resolve the community for an ownership handover notification")
			continue
		}
		params = append(params, notificationuc.PublishParams{
			UserID:     h.successor,
			Type:       notificationuc.TypeCommunityRole,
			TargetType: &targetType,
			TargetID:   &communityID,
			Message:    notificationuc.MessageInheritedCommunityOwner,
		})
	}
	if err := d.notifier.PublishBatch(ctx, params); err != nil {
		logger.Log.Error().Err(err).Int("handovers", len(params)).
			Msg("failed to publish ownership handover notifications")
	}
}

// settleCommunities は、個人情報を消す前にオーナーとしての後始末をする。
//
//   - 自分しか居ないコミュニティは消す（誰も居ないルームを残さない）。
//   - 他にメンバーが居るのに自分が唯一のオーナーなら、オーナーを引き継ぐ。
//
// 本人の退会の時点で一度引き継いでいるが、猶予の間に引き継いだ人が抜けたり、
// 自分しか居なかったコミュニティにメンバーが入ったりする。管理者による削除では
// 退会の手順を通らない。そこで消す直前にもう一度確かめる。
func (d *accountDeletion) settleCommunities(ctx context.Context, userID int64) ([]ownerHandover, error) {
	owned, roles, err := d.ownedCommunities(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, roomID := range owned {
		if len(roles[roomID]) <= 1 {
			if _, err := d.rooms.DeleteRoom(ctx, roomID); err != nil {
				return nil, err
			}
		}
	}
	return d.handOverOwnerships(ctx, userID, owned, roles)
}

// purge は userID の個人情報を消す。allow が false を返したら何もしない。
//
// allow は行をロックした後の状態で判定する。一覧を取ってから消すまでの間に、
// 本人がログインして退会を取り消しているかもしれないため。
//
// 添付の実体はコミットした後に消す。トランザクションの中で消すと、その後の失敗で
// ロールバックしたときに「DB には残っているのに実体が無い」添付ができる。
func (d *accountDeletion) purge(ctx context.Context, userID int64, allow func(*model.UserLifecycle) bool) (bool, error) {
	var keys []string
	var handovers []ownerHandover
	var purged bool
	err := d.txManager.RunInTx(ctx, func(ctx context.Context) error {
		lifecycle, err := d.users.LockUserLifecycle(ctx, userID)
		if err != nil {
			return err
		}
		if lifecycle == nil || lifecycle.Status == model.UserStatusDeleted || !allow(lifecycle) {
			return nil
		}
		if handovers, err = d.settleCommunities(ctx, userID); err != nil {
			return err
		}
		// 返信数の数え直しは、投稿が CASCADE で消える前にやる。数え直しは「この人の
		// 投稿が返信していた先」を辿るので、投稿が消えた後だと辿れない。先に論理削除
		// して数えなくし、その上で数え直す。
		if err := d.posts.DeletePostsByUserID(ctx, userID); err != nil {
			return err
		}
		if err := d.posts.RecalculateReplyCountsAffectedByUser(ctx, userID); err != nil {
			return err
		}
		if keys, err = d.media.ListStorageKeysByUploader(ctx, userID); err != nil {
			return err
		}
		if purged, err = d.users.PurgeUser(ctx, userID, d.now()); err != nil || !purged {
			return err
		}
		return d.users.DeleteActivityHistory(ctx, userID)
	})
	if err != nil {
		return false, err
	}
	if purged {
		for _, key := range keys {
			d.objects.Discard(ctx, key)
		}
		d.notifyNewOwners(ctx, handovers)
	}
	return purged, nil
}

// --- 本人の退会 ---------------------------------------------------------------

type DeleteMyAccountUseCase interface {
	Execute(ctx context.Context) (bool, error)
}

var _ DeleteMyAccountUseCase = &DeleteMyAccountInteractor{}

type DeleteMyAccountInteractor struct{ d *accountDeletion }

func NewDeleteMyAccountUseCase(deps AccountDeletionDeps) DeleteMyAccountUseCase {
	return &DeleteMyAccountInteractor{d: newAccountDeletion(deps)}
}

// Execute は本人を退会手続き中にする。猶予（model.AccountDeletionGracePeriod）が
// 切れるまでは何も消さず、その間にログインすれば取り消せる。
func (uc *DeleteMyAccountInteractor) Execute(ctx context.Context) (bool, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return false, err
	}
	var deactivated bool
	var handovers []ownerHandover
	err = uc.d.txManager.RunInTx(ctx, func(ctx context.Context) error {
		deactivated, err = uc.d.users.DeactivateUser(ctx, claims.ID, uc.d.now())
		if err != nil || !deactivated {
			return err
		}
		// 唯一のオーナーでも、そのまま抜けられるようにその場で引き継ぐ。猶予の15日間、
		// 周りから見えないオーナーしか居ないコミュニティを作らないため。
		owned, roles, err := uc.d.ownedCommunities(ctx, claims.ID)
		if err != nil {
			return err
		}
		handovers, err = uc.d.handOverOwnerships(ctx, claims.ID, owned, roles)
		return err
	})
	if err != nil {
		return false, err
	}
	uc.d.notifyNewOwners(ctx, handovers)
	return deactivated, nil
}

// --- 管理者による削除 ---------------------------------------------------------

type DeleteUserUseCase interface {
	Execute(ctx context.Context, id int64) (bool, error)
}

var _ DeleteUserUseCase = &DeleteUserInteractor{}

type DeleteUserInteractor struct{ d *accountDeletion }

func NewDeleteUserUseCase(deps AccountDeletionDeps) DeleteUserUseCase {
	return &DeleteUserInteractor{d: newAccountDeletion(deps)}
}

// Execute は管理者が利用者を削除する。猶予を置かず、その場で個人情報を消す。
// 状態は問わない（凍結中でも退会手続き中でも消す）。
func (uc *DeleteUserInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return false, err
	}
	return uc.d.purge(ctx, id, func(*model.UserLifecycle) bool { return true })
}

// --- 猶予切れ -----------------------------------------------------------------

type PurgeExpiredAccountsUseCase interface {
	// Execute は now の時点で猶予の切れた退会手続き中の利用者の個人情報を消し、
	// 消した人数を返す。1人の失敗で他の人を止めない（失敗はまとめて返す）。
	Execute(ctx context.Context, now time.Time) (int, error)
}

var _ PurgeExpiredAccountsUseCase = &PurgeExpiredAccountsInteractor{}

type PurgeExpiredAccountsInteractor struct{ d *accountDeletion }

func NewPurgeExpiredAccountsUseCase(deps AccountDeletionDeps) PurgeExpiredAccountsUseCase {
	return &PurgeExpiredAccountsInteractor{d: newAccountDeletion(deps)}
}

// purgeBatchSize は1回の一覧で引く人数。
const purgeBatchSize = 100

func (uc *PurgeExpiredAccountsInteractor) Execute(ctx context.Context, now time.Time) (int, error) {
	before := now.Add(-model.AccountDeletionGracePeriod)
	expired := func(l *model.UserLifecycle) bool { return l.GracePeriodExpired(now) }

	purged := 0
	var errs []error
	failed := map[int64]bool{}
	for {
		ids, err := uc.d.users.ListUserIDsToPurge(ctx, before, purgeBatchSize+len(failed))
		if err != nil {
			return purged, errors.Join(append(errs, err)...)
		}
		progressed := false
		for _, id := range ids {
			if failed[id] {
				continue
			}
			ok, err := uc.d.purge(ctx, id, expired)
			if err != nil {
				// 失敗した人は今回の実行ではもう試さない（同じ人で回り続けないため）。
				// 次の日次の実行でまた拾われる。
				failed[id] = true
				errs = append(errs, fmt.Errorf("purge user %d: %w", id, err))
				continue
			}
			progressed = true
			if ok {
				purged++
			}
		}
		// 一覧が上限まで埋まっていなければ、猶予の切れた人はもう残っていない。
		if len(ids) < purgeBatchSize+len(failed) || !progressed {
			break
		}
	}
	return purged, errors.Join(errs...)
}
