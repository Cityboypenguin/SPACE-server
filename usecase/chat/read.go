package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
)

// ReadReceiptService は既読位置の記録と未読件数の取得。
//
// 既読位置の置き場はルーム種別で分かれる（授業内チャットは course_room_reads、
// それ以外は room_users）ので、その振り分けをここ1箇所に閉じ込める。
type ReadReceiptService interface {
	// MarkAsRead の lastReadMessageID は「クライアントが実際に画面へ出した最後の
	// メッセージID」。nil なら従来どおりサーバが最新メッセージを既読位置にする
	// （古いクライアント互換。roomusecase.resolveReadMessageID のコメント参照）。
	MarkAsRead(ctx context.Context, roomID int64, lastReadMessageID *int64) error
	// GetReadStatus は閲覧権限を確かめたうえで既読位置と未読件数を返す。
	GetReadStatus(ctx context.Context, roomID int64) (*roomusecase.RoomReadStatus, error)
	// ReadStatusOfAuthorizedRoom は EnsureReadAccess 済みのルームについて同じものを返す
	// （権限判定を二度走らせないための口。渡してよい room の条件は実装のコメント参照）。
	ReadStatusOfAuthorizedRoom(ctx context.Context, room *model.Room) (*roomusecase.RoomReadStatus, error)
}

// ReadReceiptDeps は既読まわりが実際に使うものだけ。
type ReadReceiptDeps struct {
	// Access は GetReadStatus の閲覧権限判定に使う。MarkAsRead は別の規則なので
	// 使わない（MarkAsRead のコメント参照）。
	Access AccessPolicy

	GetRoom roomusecase.GetRoomUseCase
	// GetRoomMemberIDs は MarkAsRead の membership 判定と、DM 通知の宛先に使う。
	GetRoomMemberIDs roomusecase.GetUserIDsByRoomIDUseCase

	MarkRoomAsRead          roomusecase.MarkRoomAsReadUseCase
	MarkCourseRoomAsRead    roomusecase.MarkCourseRoomAsReadUseCase
	GetRoomReadStatus       roomusecase.GetRoomReadStatusUseCase
	GetCourseRoomReadStatus roomusecase.GetCourseRoomReadStatusUseCase

	// Events は配信・通知の出口。nil を渡すと何もしない実装が入る。
	Events EventPublisher
}

var _ ReadReceiptService = &readReceiptService{}

type readReceiptService struct {
	deps ReadReceiptDeps
}

func NewReadReceiptService(deps ReadReceiptDeps) ReadReceiptService {
	if deps.Events == nil {
		deps.Events = NoopEventPublisher{}
	}
	return &readReceiptService{deps: deps}
}

// MarkAsRead は roomID を呼び出し元の既読にする。
//
// 既読位置の置き場はルーム種別で分かれる。授業内チャットは room_users を使わない
// ので course_room_reads に、それ以外は room_users に持つ。どちらも既読位置は
// メッセージID（時刻ではなく ID を使う理由は repository.ReadPosition を参照）で、
// 保存するIDの決め方と検証は roomusecase.resolveReadMessageID に集約してある。
//
// ここで AccessPolicy.EnsureReadAccess を使わないのは意図的で、既読は「読めるか」
// ではなく「自分の既読位置を持てるか」の話。EnsureReadAccess は管理者に非メンバーの
// コミュニティの閲覧を許すが、管理者には room_users の行が無いので既読位置を
// 書く先が無い（UPDATE が0行に当たるだけ）。判定を緩めても書けないものは書けず、
// 「既読にしたのに反映されない」という分かりにくい挙動になるだけなので、統合前と
// 同じく membership を要求する。この判断は今も妥当:
//   - 非授業ルームでは membership ⊆ 閲覧可なので、EnsureReadAccess より緩く
//     なることはない（＝閲覧できない部屋を既読にはできない）。
//   - 授業ルームは EnsureReadAccess も「認証済みなら誰でも」なので同じ。
func (s *readReceiptService) MarkAsRead(ctx context.Context, roomID int64, lastReadMessageID *int64) error {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return err
	}

	room, err := s.deps.GetRoom.Execute(ctx, roomID)
	if err != nil {
		return fmt.Errorf("failed to get room")
	}

	var memberIDs []int64
	if room.Type == model.RoomTypeCourse {
		if err := s.deps.MarkCourseRoomAsRead.Execute(ctx, roomID, claims.ID, lastReadMessageID); err != nil {
			return err
		}
	} else {
		memberIDs, err = s.deps.GetRoomMemberIDs.Execute(ctx, roomID)
		if err != nil {
			return fmt.Errorf("failed to verify room membership")
		}
		if !containsInt64(memberIDs, claims.ID) {
			return errors.New("forbidden: not a member of this room")
		}
		if err := s.deps.MarkRoomAsRead.Execute(ctx, roomID, claims.ID, lastReadMessageID); err != nil {
			return err
		}
	}

	s.deps.Events.RoomMarkedAsRead(ctx, RoomMarkedAsReadEvent{
		Room:      room,
		ActorID:   claims.ID,
		MemberIDs: memberIDs,
	})
	return nil
}

// GetReadStatus は呼び出し元自身の既読位置と未読件数を返す。
//
// 閲覧権限をここで確かめる。以前は呼び出し元（Room リゾルバ）が先に
// EnsureReadAccess を通していたので表面上は安全だったが、メソッド単体では
// 非メンバーのプライベートルームの未読件数を引けてしまい、サービスの契約として
// 不完全だった。新しい呼び出し元が増えたときに権限判定ごと忘れられる形にしない。
func (s *readReceiptService) GetReadStatus(ctx context.Context, roomID int64) (*roomusecase.RoomReadStatus, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	room, err := s.deps.Access.EnsureReadAccess(ctx, roomID)
	if err != nil {
		return nil, err
	}
	return s.readStatus(ctx, room, claims.ID)
}

// ReadStatusOfAuthorizedRoom は EnsureReadAccess を通したあとのルームの既読状態を返す。
//
// room には必ず AccessPolicy の EnsureReadAccess / EnsureWriteAccess が返した値を
// 渡すこと。GetReadStatus をそのまま呼ぶと、既に閲覧権限を判定済みの呼び出し元
// （Room リゾルバ）で GetRoom と membership の取得がもう一度走り、ルーム表示の DB
// アクセスが倍になる。判定の中身は1箇所（EnsureReadAccess）に保ったまま、
// その結果を使い回すための口。
func (s *readReceiptService) ReadStatusOfAuthorizedRoom(ctx context.Context, room *model.Room) (*roomusecase.RoomReadStatus, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	return s.readStatus(ctx, room, claims.ID)
}

// readStatus は既読位置の置き場を選ぶだけの内部処理（権限判定は呼び出し側で済ませる）。
//
// どちらの経路も「相手の既読位置（PartnerLastReadAt）が入るのは DM だけ」で揃えて
// ある。授業内チャットは匿名なので常に nil（GetCourseRoomReadStatusUseCase）、
// コミュニティは相手が1人に決まらないので nil（GetRoomReadStatusUseCase が room.Type
// で判定する）。理由は roomusecase.RoomReadStatus のコメントに1箇所だけ書いてある。
func (s *readReceiptService) readStatus(ctx context.Context, room *model.Room, userID int64) (*roomusecase.RoomReadStatus, error) {
	if room.Type == model.RoomTypeCourse {
		return s.deps.GetCourseRoomReadStatus.Execute(ctx, room.ID, userID)
	}
	return s.deps.GetRoomReadStatus.Execute(ctx, room, userID)
}
