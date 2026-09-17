package graph

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/audit"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	anonusecase "github.com/Cityboypenguin/SPACE-server/usecase/anon"
	chatusecase "github.com/Cityboypenguin/SPACE-server/usecase/chat"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
)

// chatEventPubSub / chatEventSSEBroker は配信先の狭い口。
//
// *pubsub.PubSub / *sse.Broker をそのまま持つとアダプタ単体のテストが書けないので、
// 実際に呼ぶメソッドだけのインターフェースを切って受け取る（実装側は既存の型が
// そのまま満たすので、本番の配線は何も変わらない）。
type chatEventPubSub interface {
	Publish(topic string, data interface{})
}

type chatEventSSEBroker interface {
	PublishToUser(userID int64, eventType string, data map[string]any)
	PublishSyncToUser(userID int64, unreadCount int)
}

// ChatEventPublisherDeps は配信・通知アダプタが使う依存。
//
// 以前は *Resolver をまるごと持っていたため、(a) 何に依存しているかがコードから
// 読めず、(b) アダプタ単体のテストに巨大な Resolver の組み立てが要り、(c) main.go
// が「resolver を作る→resolver にサービスを差す」という相互参照になっていた。
// 実際に使うものだけを名前付きで受け取れば、配線は publisher → サービス → resolver の
// 一方向で済む。
type ChatEventPublisherDeps struct {
	PubSub                chatEventPubSub
	SSEBroker             chatEventSSEBroker
	NotificationPublisher notificationuc.NotificationPublisher

	// MembersUnreadCounts / CourseRoomUnreadCounts は未読SSEの宛先。
	// 授業ルームは room_users を使わないため母集団の引き方が別（membersUnreadCounts 参照）。
	MembersUnreadCounts    roomusecase.GetMembersUnreadCountsUseCase
	CourseRoomUnreadCounts roomusecase.GetCourseRoomUnreadCountsUseCase

	// GetMessage は引用返信の通知先（返信元の投稿者）を引くため。
	GetMessage messageusecase.GetMessageByIDUseCase
	// GetAnonymousIdentity は授業ルームの返信通知の文言に使う匿名ラベル。
	// 採番しない読み取り専用の口（採番は投稿時に送信サービス側で済んでいる）。
	GetAnonymousIdentity anonusecase.GetAnonymousIdentityUseCase

	MarkNotificationsAsReadByActor notificationuc.MarkAllAsReadByActorUseCase
	CountUnreadNotifications       notificationuc.CountUnreadUseCase
}

// chatEventPublisher は chat.EventPublisher の実装。PubSub / SSE / 通知 /
// 監査ログという「GraphQL 側だけが知っている配信先」をまとめて引き受ける。
//
// ここに置いているのは、配信内容の組み立てに GraphQL 型（gqlmodel.Message）や
// 不透明ID（opaqueid）が要るため。usecase/chat はドメインの値だけを渡してくる。
//
// ポートの契約どおり、どのメソッドも error を返さない。保存は既にコミット済みで、
// 配信の失敗でメッセージ送信を失敗させるわけにはいかないので、失敗はログに残して
// 先へ進む。ログは全て logChatDelivery を通し、あとから追える形に揃えている
// （usecase/chat/events.go のポートのコメントを参照）。
type chatEventPublisher struct {
	deps ChatEventPublisherDeps
}

var _ chatusecase.EventPublisher = &chatEventPublisher{}

// NewChatEventPublisher はチャットサービスのイベント出口を、実際の配信先へ繋ぐ。
func NewChatEventPublisher(deps ChatEventPublisherDeps) chatusecase.EventPublisher {
	return &chatEventPublisher{deps: deps}
}

// 配信・通知の失敗ログで使う stage 名。ログを grep する側が「どの経路が落ちたか」で
// 絞り込めるよう、文字列はここ1箇所に集める。
const (
	chatDeliveryUnreadBroadcast   = "unread_room_broadcast"
	chatDeliveryDMNotification    = "dm_notification"
	chatDeliveryReplyNotification = "message_reply_notification"
	chatDeliveryReplyParentLookup = "message_reply_parent_lookup"
	chatDeliveryAnonymousLabel    = "anonymous_label_lookup"
	chatDeliveryMentionNotify     = "mention_notification"
	chatDeliveryDMReadSync        = "dm_notification_read_sync"
	chatDeliveryUnreadCountSync   = "notification_unread_count_sync"
)

// logChatDelivery はベストエフォートな配信・通知の取りこぼしを、必ず同じ形で残す。
//
// Outbox を持たない以上、ここに出ないと「メッセージは保存されたのに通知だけ出て
// いない」状態に誰も気づけない。呼び出し側は room_id / message_id など後から
// 追える情報を足してから Msg すること。
func logChatDelivery(err error, stage string) *zerolog.Event {
	return logger.Log.Error().Err(err).
		Str("component", "chat_event_publisher").
		Str("delivery", stage)
}

func (p *chatEventPublisher) MessageSent(ctx context.Context, ev chatusecase.MessageSentEvent) {
	roomGraphID := encodeGraphID("room", ev.Room.ID)
	gqlMsg := toGraphMessage(ev.Message)
	p.deps.PubSub.Publish(roomGraphID+":message:added", gqlMsg)

	previewMessage := messagePreview(ev.Message.Content, ev.HasMedia)

	// 各宛先の未読カウント・最新メッセージプレビューをリアルタイム通知。
	//
	// 宛先の母集団はルーム種別で違う。授業内チャットは room_users を使わない設計
	// （誰でも閲覧でき匿名で表示する）なので、room_users を起点に数える
	// GetMembersUnreadCountsUseCase では履修者が1人も出てこず、未読の更新が誰にも
	// 届かない。授業ルームだけは「その授業を時間割に登録している利用者」を母集団に
	// する専用の経路へ振り分ける。
	if unreadCounts, err := p.membersUnreadCounts(ctx, ev.Room, ev.ActorID); err != nil {
		// 未読バッジの更新に失敗してもメッセージ送信自体は成功しているので配信は続ける。
		// ただし黙って落とすと「バッジだけ古いまま」が障害として見えないのでログに残す。
		logChatDelivery(err, chatDeliveryUnreadBroadcast).
			Int64("room_id", ev.Room.ID).
			Int64("message_id", ev.Message.ID).
			Msg("failed to load unread counts; skipping the unread_room broadcast")
	} else {
		for memberID, count := range unreadCounts {
			p.deps.SSEBroker.PublishToUser(memberID, "unread_room", map[string]any{
				"roomID":      roomGraphID,
				"unreadCount": count,
				"lastMessage": previewMessage,
			})
		}
	}

	// DM ルームの場合、相手に通知を送る
	if ev.Room.Type == model.RoomTypeDM {
		targetType := notificationuc.TargetRoom
		for _, memberID := range ev.MemberIDs {
			if memberID == ev.ActorID {
				continue
			}
			if err := p.deps.NotificationPublisher.Publish(ctx, notificationuc.PublishParams{
				UserID:     memberID,
				Type:       notificationuc.TypeDM,
				ActorID:    &ev.ActorID,
				TargetType: &targetType,
				TargetID:   &ev.Room.ID,
				Message:    previewMessage,
			}); err != nil {
				logChatDelivery(err, chatDeliveryDMNotification).
					Int64("room_id", ev.Room.ID).
					Int64("message_id", ev.Message.ID).
					Int64("recipient_id", memberID).
					Msg("failed to publish dm notification")
			}
		}
	}

	// 引用返信の通知。DM は全メッセージで既に dm 通知が飛ぶので二重通知を避けて対象外。
	// notifiedByReply には返信通知を送った相手が入り、同じ人をメンションしていても
	// メンション通知が二重にならないようにする。
	var notifiedByReply *int64
	if ev.Message.ReplyToID != nil && ev.Room.Type != model.RoomTypeDM {
		notifiedByReply = p.publishMessageReplyNotification(ctx, ev.Room, ev.Message, ev.ActorID)
	}

	p.publishMentionNotifications(ctx, ev.Room, ev.Message, ev.ActorID, notifiedByReply)
}

// membersUnreadCounts は未読SSEの宛先（利用者ID → 未読数）を、ルーム種別に応じた
// 経路で1クエリぶんまとめて引く。授業ルームは履修者が多くなりうるが、返るのは
// 利用者IDと件数だけで、配信そのものは SSE ブローカーへの非ブロッキングな
// 書き込みなので、1クエリ＋宛先ぶんのループで足りる。
func (p *chatEventPublisher) membersUnreadCounts(ctx context.Context, room *model.Room, actorID int64) (map[int64]int, error) {
	if room.Type == model.RoomTypeCourse {
		return p.deps.CourseRoomUnreadCounts.Execute(ctx, room.ID, actorID)
	}
	return p.deps.MembersUnreadCounts.Execute(ctx, room.ID, actorID)
}

func (p *chatEventPublisher) MessageUpdated(ctx context.Context, ev chatusecase.MessageUpdatedEvent) {
	if len(ev.AddedMentions) > 0 {
		// 追加分だけを持つコピーを渡す（publishMentionNotifications は
		// msg.Mentions を宛先として読む）。
		notified := *ev.Message
		notified.Mentions = ev.AddedMentions
		p.publishMentionNotifications(ctx, ev.Room, &notified, ev.ActorID, nil)
	}

	roomGraphID := encodeGraphID("room", ev.Room.ID)
	p.deps.PubSub.Publish(roomGraphID+":message:updated", toGraphMessage(ev.Message))
}

func (p *chatEventPublisher) MessageDeleted(ctx context.Context, ev chatusecase.MessageDeletedEvent) {
	audit.LogMessageDeleted(ctx, ev.RoomID, ev.MessageID)
	roomGraphID := encodeGraphID("room", ev.RoomID)
	p.deps.PubSub.Publish(roomGraphID+":message:deleted", &gqlmodel.Message{ID: encodeGraphID("message", ev.MessageID)})
}

func (p *chatEventPublisher) RoomMarkedAsRead(ctx context.Context, ev chatusecase.RoomMarkedAsReadEvent) {
	roomGraphID := encodeGraphID("room", ev.Room.ID)

	// DM ルームを既読にした際は、相手からの DM 通知も既読にする
	if ev.Room.Type == model.RoomTypeDM {
		for _, memberID := range ev.MemberIDs {
			if memberID == ev.ActorID {
				continue
			}
			if err := p.deps.MarkNotificationsAsReadByActor.Execute(ctx, ev.ActorID, string(notificationuc.TypeDM), memberID); err != nil {
				logChatDelivery(err, chatDeliveryDMReadSync).
					Int64("room_id", ev.Room.ID).
					Int64("actor_id", ev.ActorID).
					Int64("partner_id", memberID).
					Msg("failed to mark dm notifications as read")
			}
		}
		count, err := p.deps.CountUnreadNotifications.Execute(ctx, ev.ActorID)
		if err != nil {
			// 件数が引けないときはベルの同期を送らない（挙動は従来どおり）。
			// 黙って落とすと「既読にしたのにベルの数字が減らない」に気づけないので残す。
			logChatDelivery(err, chatDeliveryUnreadCountSync).
				Int64("room_id", ev.Room.ID).
				Int64("actor_id", ev.ActorID).
				Msg("failed to count unread notifications; skipping the bell sync")
		} else {
			p.deps.SSEBroker.PublishSyncToUser(ev.ActorID, int(count))
		}
	}

	// 既読の配信は相手側の既読表示のためのもの。授業内チャットは匿名なので、
	// 誰が読んだか(実ユーザーID)をルームの購読者へ配信しない。
	if ev.Room.Type != model.RoomTypeCourse {
		nowStr := time.Now().Format(timeFormat)
		p.deps.PubSub.Publish(roomGraphID+":read_status", &gqlmodel.RoomReadStatusUpdate{
			UserID:     encodeGraphID("user", ev.ActorID),
			LastReadAt: nowStr,
		})
	}

	p.deps.SSEBroker.PublishToUser(ev.ActorID, "unread_room", map[string]any{
		"roomID":      roomGraphID,
		"unreadCount": 0,
	})
}

// publishMessageReplyNotification notifies the author of the message that `reply`
// quotes. 自分自身への返信、および返信先が見つからない（削除済み）場合は何もしない。
//
// 授業内チャットは匿名なので、通知文言にはルーム内の匿名ラベルを入れ、actor_id は
// あえて保存しない。actor_id を残すと myNotifications(actorID:) や
// markAllNotificationsAsReadByActor など actor で絞り込むAPIから
// 「匿名NNN = そのユーザー」を突き合わせられてしまい、匿名性が崩れるため。
//
// 遷移先の組み立てにはルームIDと種別が要るが notifications 行は targetType/targetID の
// 1組しか持てないため、SSE には Extra で roomID/roomType を添える（GraphQL 側は
// Notification.targetMessage から辿れる）。
//
// 通知の失敗はメッセージ送信の成否に影響させない（ログのみ）。
//
// 戻り値は通知を送った相手のユーザーID（送らなかったときは nil）。
// 同じ相手をメンションしていたときにメンション通知を二重に送らないために使う。
func (p *chatEventPublisher) publishMessageReplyNotification(ctx context.Context, room *model.Room, reply *model.Message, actorID int64) *int64 {
	if reply.ReplyToID == nil {
		return nil
	}

	parent, err := p.deps.GetMessage.Execute(ctx, *reply.ReplyToID)
	if err != nil {
		// 返信元が引けないときは通知を諦める（挙動は従来どおり）。返信先が削除済みで
		// nil が返るのは正常系なので、error のときだけ残す。
		logChatDelivery(err, chatDeliveryReplyParentLookup).
			Int64("room_id", room.ID).
			Int64("message_id", reply.ID).
			Int64("reply_to_id", *reply.ReplyToID).
			Msg("failed to load the quoted message; skipping the reply notification")
		return nil
	}
	if parent == nil {
		return nil
	}
	if parent.UserID == actorID {
		return nil
	}

	message := "あなたのメッセージに返信がありました"
	notificationActorID := &actorID
	if room.Type == model.RoomTypeCourse {
		// 匿名IDは投稿時に確定済みなので、ここは採番せず読むだけ。行が引けなくても
		// 実名を出すわけにはいかないので、番号なしの「匿名」で通知する。
		label := anonymousPlaceholderLabel
		identity, err := p.deps.GetAnonymousIdentity.Execute(ctx, room.ID, actorID)
		if err != nil {
			logChatDelivery(err, chatDeliveryAnonymousLabel).
				Int64("room_id", room.ID).
				Int64("message_id", reply.ID).
				Msg("failed to resolve anonymous identity for reply notification")
		} else if identity != nil {
			label = identity.Label
		}
		message = fmt.Sprintf("%sさんがあなたのメッセージに返信しました", label)
		notificationActorID = nil
	}

	targetType := notificationuc.TargetMessage
	if err := p.deps.NotificationPublisher.Publish(ctx, notificationuc.PublishParams{
		UserID:     parent.UserID,
		Type:       notificationuc.TypeMessageReply,
		ActorID:    notificationActorID,
		TargetType: &targetType,
		TargetID:   &reply.ID,
		Message:    message,
		Extra: map[string]any{
			"roomID":   encodeGraphID("room", room.ID),
			"roomType": room.Type,
		},
	}); err != nil {
		logChatDelivery(err, chatDeliveryReplyNotification).
			Int64("room_id", room.ID).
			Int64("message_id", reply.ID).
			Int64("recipient_id", parent.UserID).
			Msg("failed to publish message reply notification")
		return nil
	}
	return &parent.UserID
}

// publishMentionNotifications はコミュニティチャットでメンションされた各ユーザーへ通知する。
//
// skipUserID には引用返信の通知を既に送った相手を渡す。返信と同時にその相手を
// メンションしても通知が2通にならないようにするため。
// 遷移先の組み立てにはルームIDと種別が要るが notifications 行は targetType/targetID の
// 1組しか持てないため、SSE には Extra で roomID/roomType を添える
// （publishMessageReplyNotification と同じ扱い）。
// 通知の失敗はメッセージ送信の成否に影響させない（ログのみ）。
func (p *chatEventPublisher) publishMentionNotifications(ctx context.Context, room *model.Room, msg *model.Message, actorID int64, skipUserID *int64) {
	if len(msg.Mentions) == 0 {
		return
	}

	targetType := notificationuc.TargetMessage
	params := make([]notificationuc.PublishParams, 0, len(msg.Mentions))
	for _, m := range msg.Mentions {
		if skipUserID != nil && m.UserID == *skipUserID {
			continue
		}
		params = append(params, notificationuc.PublishParams{
			UserID:     m.UserID,
			Type:       notificationuc.TypeMessageMention,
			ActorID:    &actorID,
			TargetType: &targetType,
			TargetID:   &msg.ID,
			Message:    "コミュニティであなたがメンションされました",
			Extra: map[string]any{
				"roomID":   encodeGraphID("room", room.ID),
				"roomType": room.Type,
			},
		})
	}
	if len(params) == 0 {
		return
	}

	if err := p.deps.NotificationPublisher.PublishBatch(ctx, params); err != nil {
		logChatDelivery(err, chatDeliveryMentionNotify).
			Int64("room_id", room.ID).
			Int64("message_id", msg.ID).
			Int("recipients", len(params)).
			Msg("failed to publish mention notifications")
	}
}

// messagePreview は一覧画面のプレビュー・通知文言に使う要約テキストを作る。
// 本文が空でも添付だけの送信はありうるので、そのときは代替文言を返す。
func messagePreview(content string, hasMedia bool) string {
	if content == "" {
		if hasMedia {
			return "[画像/ファイルを送信しました]"
		}
		return "新しいメッセージが届きました"
	}

	const maxPreviewRunes = 50
	if runes := []rune(content); utf8.RuneCountInString(content) > maxPreviewRunes {
		return string(runes[:maxPreviewRunes]) + "…"
	}
	return content
}
