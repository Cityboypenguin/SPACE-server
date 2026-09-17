package graph

import (
	"context"
	"time"
	"unicode/utf8"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/audit"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	chatusecase "github.com/Cityboypenguin/SPACE-server/usecase/chat"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

// chatEventPublisher は chat.EventPublisher の実装。PubSub / SSE / 通知 /
// 監査ログという「GraphQL 側だけが知っている配信先」をまとめて引き受ける。
//
// ここに置いているのは、配信内容の組み立てに GraphQL 型（gqlmodel.Message）や
// 不透明ID（opaqueid）が要るため。usecase/chat はドメインの値だけを渡してくる。
//
// ポートの契約どおり、どのメソッドも error を返さない。保存は既にコミット済みで、
// 配信の失敗でメッセージ送信を失敗させるわけにはいかないので、失敗はログに残して
// 先へ進む。
type chatEventPublisher struct {
	r *Resolver
}

var _ chatusecase.EventPublisher = &chatEventPublisher{}

// NewChatEventPublisher wires the chat service's event port to this resolver's
// PubSub / SSE / notification dependencies. Resolver と相互に参照するため、
// main.go では resolver を組み立てたあとに差し込む。
func NewChatEventPublisher(r *Resolver) chatusecase.EventPublisher {
	return &chatEventPublisher{r: r}
}

func (p *chatEventPublisher) MessageSent(ctx context.Context, ev chatusecase.MessageSentEvent) {
	roomGraphID := encodeGraphID("room", ev.Room.ID)
	gqlMsg := toGraphMessage(ev.Message)
	p.r.PubSub.Publish(roomGraphID+":message:added", gqlMsg)

	previewMessage := messagePreview(ev.Message.Content, ev.HasMedia)

	// 各メンバーの未読カウント・最新メッセージプレビューをリアルタイム通知
	if unreadCounts, err := p.r.GetMembersUnreadCountsUseCase.Execute(ctx, ev.Room.ID, ev.ActorID); err == nil {
		for memberID, count := range unreadCounts {
			p.r.SSEBroker.PublishToUser(memberID, "unread_room", map[string]any{
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
			if err := p.r.NotificationPublisher.Publish(ctx, notificationuc.PublishParams{
				UserID:     memberID,
				Type:       notificationuc.TypeDM,
				ActorID:    &ev.ActorID,
				TargetType: &targetType,
				TargetID:   &ev.Room.ID,
				Message:    previewMessage,
			}); err != nil {
				logger.Log.Error().Err(err).Msg("failed to publish dm notification")
			}
		}
	}

	// 引用返信の通知。DM は全メッセージで既に dm 通知が飛ぶので二重通知を避けて対象外。
	// notifiedByReply には返信通知を送った相手が入り、同じ人をメンションしていても
	// メンション通知が二重にならないようにする。
	var notifiedByReply *int64
	if ev.Message.ReplyToID != nil && ev.Room.Type != model.RoomTypeDM {
		notifiedByReply = p.r.publishMessageReplyNotification(ctx, ev.Room, ev.Message, ev.ActorID)
	}

	p.r.publishMentionNotifications(ctx, ev.Room, ev.Message, ev.ActorID, notifiedByReply)
}

func (p *chatEventPublisher) MessageUpdated(ctx context.Context, ev chatusecase.MessageUpdatedEvent) {
	if len(ev.AddedMentions) > 0 {
		// 追加分だけを持つコピーを渡す（publishMentionNotifications は
		// msg.Mentions を宛先として読む）。
		notified := *ev.Message
		notified.Mentions = ev.AddedMentions
		p.r.publishMentionNotifications(ctx, ev.Room, &notified, ev.ActorID, nil)
	}

	roomGraphID := encodeGraphID("room", ev.Room.ID)
	p.r.PubSub.Publish(roomGraphID+":message:updated", toGraphMessage(ev.Message))
}

func (p *chatEventPublisher) MessageDeleted(ctx context.Context, ev chatusecase.MessageDeletedEvent) {
	audit.LogMessageDeleted(ctx, ev.RoomID, ev.MessageID)
	roomGraphID := encodeGraphID("room", ev.RoomID)
	p.r.PubSub.Publish(roomGraphID+":message:deleted", &gqlmodel.Message{ID: encodeGraphID("message", ev.MessageID)})
}

func (p *chatEventPublisher) RoomMarkedAsRead(ctx context.Context, ev chatusecase.RoomMarkedAsReadEvent) {
	roomGraphID := encodeGraphID("room", ev.Room.ID)

	// DM ルームを既読にした際は、相手からの DM 通知も既読にする
	if ev.Room.Type == model.RoomTypeDM {
		for _, memberID := range ev.MemberIDs {
			if memberID == ev.ActorID {
				continue
			}
			if err := p.r.MarkAllAsReadByActorUseCase.Execute(ctx, ev.ActorID, string(notificationuc.TypeDM), memberID); err != nil {
				logger.Log.Error().Err(err).Msg("failed to mark dm notifications as read")
			}
		}
		if count, err := p.r.CountUnreadUseCase.Execute(ctx, ev.ActorID); err == nil {
			p.r.SSEBroker.PublishSyncToUser(ev.ActorID, int(count))
		}
	}

	// 既読の配信は相手側の既読表示のためのもの。授業内チャットは匿名なので、
	// 誰が読んだか(実ユーザーID)をルームの購読者へ配信しない。
	if ev.Room.Type != model.RoomTypeCourse {
		nowStr := time.Now().Format(timeFormat)
		p.r.PubSub.Publish(roomGraphID+":read_status", &gqlmodel.RoomReadStatusUpdate{
			UserID:     encodeGraphID("user", ev.ActorID),
			LastReadAt: nowStr,
		})
	}

	p.r.SSEBroker.PublishToUser(ev.ActorID, "unread_room", map[string]any{
		"roomID":      roomGraphID,
		"unreadCount": 0,
	})
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
