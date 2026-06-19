package model

import "time"

type Notification struct {
	ID         int64
	UserID     int64
	Type       string
	ActorID    *int64
	TargetType *string
	TargetID   *int64
	Message    string
	IsRead     bool
	CreatedAt  time.Time
}

type CreateNotificationParam struct {
	UserID     int64
	Type       string
	ActorID    *int64
	TargetType *string
	TargetID   *int64
	Message    string
}

func NewNotification(param CreateNotificationParam) *Notification {
	return &Notification{
		UserID:     param.UserID,
		Type:       param.Type,
		ActorID:    param.ActorID,
		TargetType: param.TargetType,
		TargetID:   param.TargetID,
		Message:    param.Message,
		IsRead:     false,
		CreatedAt:  time.Now(),
	}
}

// NotificationGroup は、送信者・対象単位（DMの送信者など）で集計した通知の代表行。
// 「最新の1件の内容」と「件数・未読数」をDB側のGROUP BYで計算し、クライアントに
// 全件を送らずに済むようにするための集計結果。
type NotificationGroup struct {
	Type        string
	ActorID     *int64
	TargetType  *string
	TargetID    *int64
	Message     string
	LatestID    int64
	CreatedAt   time.Time
	Count       int
	UnreadCount int
}
