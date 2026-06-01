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
)

type TargetType string

const (
	TargetPost      TargetType = "post"
	TargetRoom      TargetType = "room"
	TargetCommunity TargetType = "community"
)

type PublishParams struct {
	UserID     int64
	Type       NotificationType
	ActorID    *int64
	TargetType *TargetType
	TargetID   *int64
	Message    string
}

// UserEventDelivery はリアルタイムイベントをユーザーへ配信するポート。
// usecase 層はこのインターフェースに依存し、具体的な配信手段（SSE 等）を知らない。
type UserEventDelivery interface {
	PublishToUser(userID int64, eventType string, data map[string]any)
}

// NotificationPublisher は通知を DB に保存し、接続中のユーザーへリアルタイム配信する。
type NotificationPublisher interface {
	Publish(ctx context.Context, params PublishParams) error
}

type notificationPublisher struct {
	repo     repository.NotificationRepository
	delivery UserEventDelivery
}

func NewNotificationPublisher(repo repository.NotificationRepository, delivery UserEventDelivery) NotificationPublisher {
	return &notificationPublisher{repo: repo, delivery: delivery}
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
		"id":        opaqueid.Encode("notification", n.ID),
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

	p.delivery.PublishToUser(params.UserID, "notification", data)
	return nil
}
