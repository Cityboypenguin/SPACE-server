package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/audit"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

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

// ListMessagesInput はメッセージ一覧の取得条件。AroundID を指定すると
// そのメッセージを中心に前後を取り、Cursor は無視される。
type ListMessagesInput struct {
	RoomID   int64
	Limit    int
	Cursor   repository.MessageCursor
	AroundID *int64
}

func (s *service) SendMessage(ctx context.Context, in SendMessageInput) (*model.Message, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	access, err := s.ensureWriteAccess(ctx, claims, in.RoomID)
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

	msg, err := s.deps.SendMessage.Execute(ctx, in.RoomID, claims.ID, in.Content, in.MediaInputs, in.ReplyToID, mentions)
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

func (s *service) UpdateMessage(ctx context.Context, in UpdateMessageInput) (*model.Message, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	isAdmin := authz.IsAdminRole(claims.Role)

	existing, err := s.loadMessageInRoom(ctx, in.RoomID, in.MessageID)
	if err != nil {
		return nil, err
	}
	if !existing.CanBeEditedBy(claims.ID, isAdmin) {
		audit.LogDenied(ctx, "update_message", "message", in.MessageID, "not owner")
		return nil, errors.New("forbidden: can only update your own messages")
	}

	room, err := s.deps.GetRoom.Execute(ctx, existing.RoomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get room")
	}
	// 授業内チャットは送信と同じく、履修をやめた授業・終了した学期では編集させない。
	if !isAdmin && room.Type == model.RoomTypeCourse {
		if err := s.deps.CheckRoomWritable.Execute(ctx, existing.RoomID); err != nil {
			return nil, err
		}
	}

	// 編集後の本文でメンションを解決し直す。本文から消えたメンションは保存側で消える。
	mentions, err := s.deps.ResolveMentions.Execute(ctx, existing.RoomID, existing.UserID, in.Content, in.MentionUserIDs)
	if err != nil {
		return nil, err
	}

	// 既にメンション済みの相手には再通知しないよう、編集前のメンションを控えておく。
	alreadyNotified := make(map[int64]struct{})
	if before, err := s.deps.ListMentions.Execute(ctx, []int64{in.MessageID}); err == nil {
		for _, m := range before[in.MessageID] {
			alreadyNotified[m.UserID] = struct{}{}
		}
	}

	msg, err := s.deps.UpdateMessage.Execute(ctx, in.MessageID, model.UpdateMessageParam{Content: &in.Content}, mentions)
	if err != nil {
		return nil, err
	}

	// 編集で新しく追加されたメンションだけ通知する。
	added := make([]*model.Mention, 0, len(msg.Mentions))
	for _, m := range msg.Mentions {
		if _, ok := alreadyNotified[m.UserID]; ok {
			continue
		}
		added = append(added, m)
	}

	s.deps.Events.MessageUpdated(ctx, MessageUpdatedEvent{
		Room:          room,
		Message:       msg,
		ActorID:       existing.UserID,
		AddedMentions: added,
	})
	return msg, nil
}

func (s *service) DeleteMessage(ctx context.Context, in DeleteMessageInput) (bool, error) {
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
		// 下の授業ルーム判定にルーム種別が要るので、引けないときは安全側に倒す。
		return false, fmt.Errorf("failed to get room")
	}

	// 本人・管理者かはメッセージ自身が判定する。コミュニティのオーナーによる
	// 他人のメッセージ削除は room の役割を見ないと決まらないので、ここで重ねて確認する。
	if !msg.CanBeDeletedBy(claims.ID, isAdmin) {
		if room.Type != model.RoomTypeCommunity {
			return false, errors.New("forbidden: can only delete your own messages")
		}
		role, err := s.deps.GetRoomUserRole.Execute(ctx, msg.RoomID, claims.ID)
		if err != nil || role != model.RoomUserRoleOwner {
			return false, errors.New("forbidden: can only delete your own messages")
		}
	}

	// 授業内チャットは編集と同じく、終了した学期・履修解除後は削除もできない。
	// 質問・投票は RequireWritableCourseRoomUseCase 経由で既に「終了学期／履修解除後は
	// 編集も削除も不可」になっており、同じ授業内の発言なのにメッセージだけ消せるのは
	// 一貫しないため、そちらへ揃えた。管理者は従来どおり削除できる。
	if !isAdmin && room.Type == model.RoomTypeCourse {
		if err := s.deps.CheckRoomWritable.Execute(ctx, msg.RoomID); err != nil {
			return false, err
		}
	}

	deleted, err := s.deps.DeleteMessage.Execute(ctx, msg.ID, claims.ID)
	if err != nil {
		return false, err
	}
	if deleted {
		// 配信先はメッセージ実体の RoomID から作る（引数の roomID ではない）。
		s.deps.Events.MessageDeleted(ctx, MessageDeletedEvent{RoomID: msg.RoomID, MessageID: msg.ID})
	}
	return deleted, nil
}

func (s *service) ListMessages(ctx context.Context, in ListMessagesInput) (*repository.MessagePage, error) {
	if _, err := s.EnsureReadAccess(ctx, in.RoomID); err != nil {
		return nil, err
	}

	if in.AroundID != nil {
		return s.deps.ListMessagesAround.Execute(ctx, in.RoomID, *in.AroundID, in.Limit)
	}
	return s.deps.ListMessages.Execute(ctx, repository.MessageQuery{
		RoomID: in.RoomID,
		Limit:  in.Limit,
		Cursor: in.Cursor,
	})
}

// loadMessageInRoom は「呼び出し側が指定した roomID」と「メッセージが実際に属する
// ルーム」を照合したうえでメッセージを返す。
//
// 照合が無いと、別ルームのメッセージIDを渡して自分の居るルームの購読者へ
// その内容を配信させる（あるいは別ルームの topic に publish させる）ことが
// できてしまう。「存在しない」と「別ルームにある」でエラーを分けると、その
// メッセージIDが実在するかどうかを外から測れてしまうので、どちらも
// "message not found" に寄せている。
func (s *service) loadMessageInRoom(ctx context.Context, roomID, messageID int64) (*model.Message, error) {
	msg, err := s.deps.GetMessage.Execute(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get message")
	}
	if msg == nil || !msg.IsInRoom(roomID) {
		return nil, apperr.NotFound("message not found")
	}
	return msg, nil
}
