package notification

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/opaqueid"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type NotificationType string

const (
	TypeFavorite      NotificationType = "favorite"
	TypeReply         NotificationType = "reply"
	TypeDM            NotificationType = "dm"
	TypeCommunityKick NotificationType = "community_kick"
	TypeCommunityRole NotificationType = "community_role"
	TypeAnnouncement  NotificationType = "announcement"
	TypeFollow        NotificationType = "follow"
	// TypeMessageReply はチャット内の引用返信。投稿への返信 (TypeReply) とは
	// 遷移先が異なる（ルームを開いて該当メッセージへジャンプする）ため別タイプにしている。
	TypeMessageReply NotificationType = "message_reply"
	// TypeMention は投稿本文での @accountID メンション。遷移先は投稿。
	TypeMention NotificationType = "mention"
	// TypeMessageMention はコミュニティチャットでの @表示名 メンション。
	// 返信と同じく遷移先がルーム内の該当メッセージなので、投稿のメンションとは別タイプにしている。
	TypeMessageMention NotificationType = "message_mention"
)

type TargetType string

const (
	TargetPost         TargetType = "post"
	TargetRoom         TargetType = "room"
	TargetCommunity    TargetType = "community"
	TargetAnnouncement TargetType = "announcement"
	TargetMessage      TargetType = "message"
)

type PublishParams struct {
	UserID     int64
	Type       NotificationType
	ActorID    *int64
	TargetType *TargetType
	TargetID   *int64
	Message    string
	// Extra はリアルタイム配信のペイロードにだけ載せる追加フィールド。
	// DB の notifications 行は TargetType/TargetID の1組しか持てないため、
	// 遷移先の組み立てに足りない情報（返信通知のルームIDなど）をここで補う。
	Extra map[string]any
}

// UserEventDelivery はリアルタイムイベントをユーザーへ配信するポート。
// usecase 層はこのインターフェースに依存し、具体的な配信手段（SSE 等）を知らない。
type UserEventDelivery interface {
	PublishToUser(userID int64, eventType string, data map[string]any)
}

// NotificationPublisher は通知を DB に保存し、接続中のユーザーへリアルタイム配信する。
type NotificationPublisher interface {
	Publish(ctx context.Context, params PublishParams) error
	PublishBatch(ctx context.Context, params []PublishParams) error
}

type notificationPublisher struct {
	repo     repository.NotificationRepository
	delivery UserEventDelivery
}

func NewNotificationPublisher(repo repository.NotificationRepository, delivery UserEventDelivery) NotificationPublisher {
	return &notificationPublisher{repo: repo, delivery: delivery}
}

// deliveryData builds the realtime payload for one notification, applying the
// per-notification masking (HideActor) and extra fields from params.
func deliveryData(n *model.Notification, params PublishParams) map[string]any {
	data := map[string]any{
		"id":        opaqueid.Encode("notification", n.ID),
		"type":      n.Type,
		"message":   n.Message,
		"isRead":    false,
		"createdAt": n.CreatedAt.Format(time.RFC3339),
	}
	if n.ActorID != nil {
		data["actorID"] = opaqueid.Encode("user", *n.ActorID)
	}
	if n.TargetType != nil {
		data["targetType"] = *n.TargetType
	}
	if n.TargetID != nil {
		data["targetID"] = opaqueid.Encode(*n.TargetType, *n.TargetID)
	}
	for k, v := range params.Extra {
		data[k] = v
	}
	return data
}

func (p *notificationPublisher) PublishBatch(ctx context.Context, params []PublishParams) error {
	if len(params) == 0 {
		return nil
	}
	ns := make([]*model.Notification, 0, len(params))
	for _, param := range params {
		var targetTypeStr *string
		if param.TargetType != nil {
			t := string(*param.TargetType)
			targetTypeStr = &t
		}
		ns = append(ns, model.NewNotification(model.CreateNotificationParam{
			UserID:     param.UserID,
			Type:       string(param.Type),
			ActorID:    param.ActorID,
			TargetType: targetTypeStr,
			TargetID:   param.TargetID,
			Message:    param.Message,
		}))
	}

	if err := p.repo.SaveBatch(ctx, ns); err != nil {
		return err
	}

	for i, n := range ns {
		p.delivery.PublishToUser(params[i].UserID, "notification", deliveryData(n, params[i]))
	}
	return nil
}

func (p *notificationPublisher) Publish(ctx context.Context, params PublishParams) error {
	var targetTypeStr *string
	if params.TargetType != nil {
		t := string(*params.TargetType)
		targetTypeStr = &t
	}

	n := model.NewNotification(model.CreateNotificationParam{
		UserID:     params.UserID,
		Type:       string(params.Type),
		ActorID:    params.ActorID,
		TargetType: targetTypeStr,
		TargetID:   params.TargetID,
		Message:    params.Message,
	})

	if err := p.repo.Save(ctx, n); err != nil {
		return err
	}

	p.delivery.PublishToUser(params.UserID, "notification", deliveryData(n, params))
	return nil
}
