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
