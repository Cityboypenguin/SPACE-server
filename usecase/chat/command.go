package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/audit"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	anonusecase "github.com/Cityboypenguin/SPACE-server/usecase/anon"
	mediausecase "github.com/Cityboypenguin/SPACE-server/usecase/media"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
)

// MessageCommandService はメッセージを書き換える操作（送信・編集・削除）。
//
// 受け持つのは「権限判定を通す → メンションを解決する → 匿名IDを確定させる →
// 保存する → 配信・通知を出す」という順序で、判定そのものは AccessPolicy、保存
// そのものは MessageWriters に居る。読み取りは MessageQueryService 側。
type MessageCommandService interface {
	SendMessage(ctx context.Context, in SendMessageInput) (*model.Message, error)
	UpdateMessage(ctx context.Context, in UpdateMessageInput) (*model.Message, error)
	DeleteMessage(ctx context.Context, in DeleteMessageInput) (bool, error)
}

// MessageCommandDeps は書き込み経路が実際に使うものだけ。
type MessageCommandDeps struct {
	// Access は権限判定。自前で membership やブロックを見に行かないこと。
	Access AccessPolicy

	// GetRoom は編集・削除で「メッセージが実際に属するルーム」を引くため
	// （呼び出し元が名乗った roomID ではなく実体を使う。loadMessageInRoom 参照）。
	GetRoom roomusecase.GetRoomUseCase
	// GetRoomUserRole はコミュニティのオーナーによる他人のメッセージ削除の判定。
	GetRoomUserRole roomusecase.GetRoomUserRoleUseCase

	GetMessage messageusecase.GetMessageByIDUseCase
	// Writers は保存処理。認可を通ったあとにだけ触れる形になっている（writers.go 参照）。
	Writers MessageWriters

	ResolveMentions messageusecase.ResolveMentionsUseCase
	// ListMentions は編集前後のメンション差分を取るため（再通知の抑止）。
	ListMentions messageusecase.ListMentionsByMessageIDsUseCase

	// ListMessageMedia は編集時に「添付が残っているか」を見るためだけに使う。
	// 添付は Message 集約の外にあるので、集約をまたぐ不変条件（本文も添付も空の
	// メッセージを作らせない）を守れるのは添付を引けるこの層だけ。
	ListMessageMedia mediausecase.ListMediaByMessageIDsUseCase

	// GetOrCreateAnonymousIdentity は授業ルームへの投稿時に匿名IDを確定させる。
	GetOrCreateAnonymousIdentity anonusecase.GetOrCreateAnonymousIdentityUseCase

	// Events は配信・通知の出口。nil を渡すと何もしない実装が入る。
	Events EventPublisher
}

var _ MessageCommandService = &messageCommandService{}

type messageCommandService struct {
	deps MessageCommandDeps
}

func NewMessageCommandService(deps MessageCommandDeps) MessageCommandService {
	if deps.Events == nil {
		deps.Events = NoopEventPublisher{}
	}
	return &messageCommandService{deps: deps}
}

// SendMessageInput はメッセージ送信の入力。ID は全てデコード済みの数値で受け取る
// （GraphQL ID のデコードはリゾルバの仕事）。
type SendMessageInput struct {
	RoomID      int64
	Content     string
	MediaInputs []model.MediaInput
	// MentionUserIDs はクライアントが申告したメンション先。ここでは検証せず、
	// ResolveMentionsUseCase に渡して「そのルームでメンションが成立するか」
	// （message.MentionsSupported: コミュニティのみ）から判定させる。
	MentionUserIDs []int64
	ReplyToID      *int64
}

type UpdateMessageInput struct {
	RoomID         int64
	MessageID      int64
	Content        string
	MentionUserIDs []int64
}

type DeleteMessageInput struct {
	RoomID    int64
	MessageID int64
}

func (s *messageCommandService) SendMessage(ctx context.Context, in SendMessageInput) (*model.Message, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	access, err := s.deps.Access.ensureWriteAccessFor(ctx, claims, in.RoomID)
	if err != nil {
		return nil, err
	}

	// メンションが成立するルームか（コミュニティのみ）の判定は ResolveMentions 側に
	// あり、ここは検証済みの結果だけを保存に渡す。リゾルバから ResolveMentions を
	// 直接呼ぶ経路は無くしてあるので、別経路から実名メンションを通す余地は無い。
	mentions, err := s.deps.ResolveMentions.Execute(ctx, in.RoomID, claims.ID, in.Content, in.MentionUserIDs)
	if err != nil {
		return nil, err
	}

	if err := s.ensureAnonymousIdentity(ctx, access.Room, claims.ID); err != nil {
		return nil, err
	}

	msg, err := s.deps.Writers.send.Execute(ctx, in.RoomID, claims.ID, in.Content, in.MediaInputs, in.ReplyToID, mentions)
	if err != nil {
		return nil, err
	}

	s.deps.Events.MessageSent(ctx, MessageSentEvent{
		Room:      access.Room,
		Message:   msg,
		ActorID:   claims.ID,
		MemberIDs: access.MemberIDs,
		HasMedia:  len(in.MediaInputs) > 0,
	})
	return msg, nil
}

func (s *messageCommandService) UpdateMessage(ctx context.Context, in UpdateMessageInput) (*model.Message, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	isAdmin := authz.IsAdminRole(claims.Role)

	existing, err := s.loadMessageInRoom(ctx, in.RoomID, in.MessageID)
	if err != nil {
		return nil, err
	}

	room, err := s.deps.GetRoom.Execute(ctx, existing.RoomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get room")
	}

	// ルームを操作してよいかが先。所有権より前に見るのは、非メンバーに
	// 「そのメッセージが自分のものか」を試させないため。
	// 授業の学期・履修判定もここに含まれる（ensureMutateAccess の表を参照）。
	if err := s.deps.Access.ensureMutateAccess(ctx, claims, room); err != nil {
		return nil, err
	}
	if !existing.CanBeEditedBy(claims.ID, isAdmin) {
		audit.LogDenied(ctx, "update_message", "message", in.MessageID, "not owner")
		return nil, errors.New("forbidden: can only update your own messages")
	}

	// 本文を空にする編集は、添付が1枚も無いときだけ弾く
	// （「本文も添付も無いメッセージは存在しない」を編集経路でも守る）。
	if err := s.ensureMessageNotEmptied(ctx, existing, in.Content); err != nil {
		return nil, err
	}

	// 編集後の本文でメンションを解決し直す。本文から消えたメンションは保存側で消える。
	mentions, err := s.deps.ResolveMentions.Execute(ctx, existing.RoomID, existing.UserID, in.Content, in.MentionUserIDs)
	if err != nil {
		return nil, err
	}

	// 既にメンション済みの相手には再通知しないよう、編集前のメンションを控えておく。
	//
	// 取得に失敗したら差分が出せない。以前はエラーを捨てて空の集合で続けていたため、
	// 編集前から居たメンションまで「新規追加」に見え、同じ相手へ再通知していた。
	// 保存は続けてよい（本文の編集自体は通知と無関係に成立する）が、通知は止める。
	// 送りすぎは取り消せないのに対し、送らなかった分は本文を見れば分かるので、
	// 分からないときは送らない側に倒す。
	alreadyNotified := make(map[int64]struct{})
	mentionDiffKnown := true
	before, err := s.deps.ListMentions.Execute(ctx, []int64{in.MessageID})
	if err != nil {
		logChatCommand(err).
			Int64("room_id", existing.RoomID).
			Int64("message_id", in.MessageID).
			Msg("failed to load mentions before edit; suppressing mention notifications for this edit")
		mentionDiffKnown = false
	}
	for _, m := range before[in.MessageID] {
		alreadyNotified[m.UserID] = struct{}{}
	}

	msg, err := s.deps.Writers.update.Execute(ctx, in.MessageID, model.UpdateMessageParam{Content: &in.Content}, mentions)
	if err != nil {
		return nil, err
	}

	// 編集で新しく追加されたメンションだけ通知する。
	// 差分が取れなかった編集では 1件も通知しない（上のコメントの通り）。
	added := make([]*model.Mention, 0, len(msg.Mentions))
	if mentionDiffKnown {
		for _, m := range msg.Mentions {
			if _, ok := alreadyNotified[m.UserID]; ok {
				continue
			}
			added = append(added, m)
		}
	}

	s.deps.Events.MessageUpdated(ctx, MessageUpdatedEvent{
		Room:          room,
		Message:       msg,
		ActorID:       existing.UserID,
		AddedMentions: added,
	})
	return msg, nil
}

func (s *messageCommandService) DeleteMessage(ctx context.Context, in DeleteMessageInput) (bool, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return false, err
	}
	isAdmin := authz.IsAdminRole(claims.Role)

	msg, err := s.loadMessageInRoom(ctx, in.RoomID, in.MessageID)
	if err != nil {
		return false, err
	}

	room, err := s.deps.GetRoom.Execute(ctx, msg.RoomID)
	if err != nil {
		// 下のルーム判定にルーム種別が要るので、引けないときは安全側に倒す。
		return false, fmt.Errorf("failed to get room")
	}

	// ルームを操作してよいかが先。退出・キックされた利用者は、自分の過去の
	// メッセージであっても削除できない（所有権判定の前に落とす）。
	// 授業内チャットの「終了した学期・履修解除後は削除もできない」判定もここに入る。
	// 質問・投票は RequireWritableCourseRoomUseCase 経由で既に同じ規則なので、
	// 同じ授業内の発言なのにメッセージだけ消せる、という食い違いを作らない。
	if err := s.deps.Access.ensureMutateAccess(ctx, claims, room); err != nil {
		return false, err
	}

	// 本人・管理者かはメッセージ自身が判定する。コミュニティのオーナーによる
	// 他人のメッセージ削除は room の役割を見ないと決まらないので、ここで重ねて確認する。
	// オーナーは room_users の行（role='owner'）を持つ＝必ずメンバーなので、
	// 上の membership 判定で落ちることはない。
	if !msg.CanBeDeletedBy(claims.ID, isAdmin) {
		if room.Type != model.RoomTypeCommunity {
			return false, errors.New("forbidden: can only delete your own messages")
		}
		role, err := s.deps.GetRoomUserRole.Execute(ctx, msg.RoomID)
		if err != nil {
			// 役割が引けないときは安全側（拒否）に倒す。挙動は従来どおりだが、
			// オーナーの削除が DB 障害で「権限がない」に化けているのと、本当に
			// オーナーでないのとが区別できなくなるのでログに残す。
			logChatCommand(err).
				Int64("room_id", msg.RoomID).
				Int64("message_id", msg.ID).
				Msg("failed to load the room role; denying the delete")
		}
		if err != nil || role != model.RoomUserRoleOwner {
			return false, errors.New("forbidden: can only delete your own messages")
		}
	}

	deleted, err := s.deps.Writers.delete.Execute(ctx, msg.ID, claims.ID)
	if err != nil {
		return false, err
	}
	if deleted {
		// 配信先はメッセージ実体の RoomID から作る（引数の roomID ではない）。
		s.deps.Events.MessageDeleted(ctx, MessageDeletedEvent{RoomID: msg.RoomID, MessageID: msg.ID})
	}
	return deleted, nil
}

// ensureAnonymousIdentity は授業ルームへの書き込み時に匿名ID（匿名NNN）を確定させる。
//
// 以前は表示時（messageResolver.User など）に採番していたため、(a) 読むだけの
// クエリが DB に行を作る副作用を持ち、(b) 番号が「そのルームで初めて投稿した順」
// ではなく「初めて誰かの画面に出た順」になりえた。投稿時に採番すれば番号は
// 投稿順に固定され、表示側は読み取りだけで済む。
//
// 保存トランザクションの外で呼んでいるのは、採番がルームごとのカウンタ
// (room_anonymous_sequences) を1文で進める独立した処理で、メッセージ保存の
// トランザクションに巻き込む必要が無いため。保存が失敗しても残るのは
// 「まだ投稿していない人の番号」だけで、その人が次に投稿したときに同じ行が
// 再利用されるので実害はない（カウンタは進んだままだが、番号の欠番は許容する。
// 詳細は infra/mysql/room_anonymous_identity_repository.go の GetOrCreate）。
//
// 権限判定ではなく「投稿という行為に伴う副作用」なので、AccessPolicy ではなく
// 送信サービス側に置いている。
func (s *messageCommandService) ensureAnonymousIdentity(ctx context.Context, room *model.Room, userID int64) error {
	if room == nil || room.Type != model.RoomTypeCourse {
		return nil
	}
	_, err := s.deps.GetOrCreateAnonymousIdentity.Execute(ctx, room.ID, userID)
	return err
}

// ensureMessageNotEmptied は「本文も添付も無いメッセージは存在しない」という
// Message の不変条件を、編集経路でも守る。
//
// 生成時は model.NewMessage が CreateMessageParam.MediaKeys を見て弾いている。
// ところが添付は Message 集約の外（media / message_media）にあるため、保存だけを
// 担う messagestore からは添付の有無が見えず、UpdateMessage は本文の文字数しか
// 検証できない。その結果、添付なしのメッセージを空本文へ編集して「本文も添付も
// 無いメッセージ」を作れてしまっていた。
//
// どの層が何を守るかの線引き:
//   - model.Message … 自分の持ち物（本文）だけで決まる不変条件
//   - messagestore  … 集約の外を知らないまま保存する（文字数上限などの検証まで）
//   - usecase/chat  … 添付＝集約の外を引ける層。集約をまたぐ不変条件はここで守る
//
// 添付を Message 集約に取り込めば model 側に閉じられるが、メッセージ一覧の取得が
// 常に添付を伴う形になり影響が大きい。いまは「添付の有無を知っている層で守る」を選ぶ。
//
// 文言は生成時（model.NewMessage）と同じ "content or media is required" に揃える。
// 利用者から見て同じ拒否理由なので、経路で言い方が変わらないようにする。
func (s *messageCommandService) ensureMessageNotEmptied(ctx context.Context, existing *model.Message, content string) error {
	if strings.TrimSpace(content) != "" {
		return nil
	}

	mediaByMessage, err := s.deps.ListMessageMedia.Execute(ctx, []int64{existing.ID})
	if err != nil {
		// 添付の有無が分からないまま空本文を通すと不変条件を破りうるので、
		// 編集そのものを失敗させる（握りつぶして保存する方が危険）。
		return fmt.Errorf("failed to check message attachments")
	}
	if len(mediaByMessage[existing.ID]) > 0 {
		return nil
	}
	return apperr.InvalidInput("content or media is required")
}

// loadMessageInRoom は「呼び出し側が指定した roomID」と「メッセージが実際に属する
// ルーム」を照合したうえでメッセージを返す。
//
// 照合が無いと、別ルームのメッセージIDを渡して自分の居るルームの購読者へ
// その内容を配信させる（あるいは別ルームの topic に publish させる）ことが
// できてしまう。「存在しない」と「別ルームにある」でエラーを分けると、その
// メッセージIDが実在するかどうかを外から測れてしまうので、どちらも
// "message not found" に寄せている。
func (s *messageCommandService) loadMessageInRoom(ctx context.Context, roomID, messageID int64) (*model.Message, error) {
	msg, err := s.deps.GetMessage.Execute(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get message")
	}
	if msg == nil || !msg.IsInRoom(roomID) {
		return nil, apperr.NotFound("message not found")
	}
	return msg, nil
}
