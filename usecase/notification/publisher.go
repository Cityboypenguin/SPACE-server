package notification

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// NotificationType は通知の種別を表す。新しい発火点を追加する際はここに定数を追加する。
type NotificationType string

const (
	TypeFavorite      NotificationType = "favorite"
	TypeReply         NotificationType = "reply"
	TypeDM            NotificationType = "dm"
	TypeCommunityKick NotificationType = "community_kick"
	TypeCommunityRole NotificationType = "community_role"
)

// TargetType は通知が指す対象の種別を表す。
type TargetType string

const (
	TargetPost      TargetType = "post"
	TargetRoom      TargetType = "room"
	TargetCommunity TargetType = "community"
)

// PublishParams は通知を発行する際のパラメータ。
type PublishParams struct {
	UserID     int64            // 通知の受信者
	Type       NotificationType // 通知種別
	ActorID    *int64           // 通知を発生させたユーザー（省略可）
	TargetType *TargetType      // 対象の種別（省略可）
	TargetID   *int64           // 対象のID（省略可）
	Message    string           // 通知メッセージ
}

// NotificationPublisher は通知を DB に保存し SSE でリアルタイム配信するインターフェース。
// 新しい発火点では Publish を1行呼ぶだけでよい。
type NotificationPublisher interface {
	Publish(ctx context.Context, params PublishParams) error
}

type notificationPublisher struct {
	repo repository.NotificationRepository
	hub  *sse.Hub
}

func NewNotificationPublisher(repo repository.NotificationRepository, hub *sse.Hub) NotificationPublisher {
	return &notificationPublisher{repo: repo, hub: hub}
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

	data := map[string]any{
		"id":        n.ID,
		"type":      n.Type,
		"message":   n.Message,
		"isRead":    false,
		"createdAt": n.CreatedAt.Format(time.RFC3339),
	}
	if n.ActorID != nil {
		data["actorID"] = *n.ActorID
	}
	if n.TargetType != nil {
		data["targetType"] = *n.TargetType
	}
	if n.TargetID != nil {
		data["targetID"] = *n.TargetID
	}

	p.hub.PublishToUser(params.UserID, "notification", data)
	return nil
}
