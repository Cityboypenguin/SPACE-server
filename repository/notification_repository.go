package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type NotificationRepository interface {
	Save(ctx context.Context, n *model.Notification) error
	SaveBatch(ctx context.Context, ns []*model.Notification) error
	ListByUserID(ctx context.Context, userID int64, limit, offset int) ([]*model.Notification, int, error)
	GetByID(ctx context.Context, id int64, userID int64) (*model.Notification, error)
	ListGroupedByUserID(ctx context.Context, userID int64, limit, offset int) ([]*model.NotificationGroup, int, error)
	ListByActor(ctx context.Context, userID int64, notifType string, actorID int64, limit, offset int) ([]*model.Notification, int, error)
	MarkAsRead(ctx context.Context, id int64, userID int64) error
	MarkAllAsRead(ctx context.Context, userID int64) error
	MarkAllAsReadByActor(ctx context.Context, userID int64, notifType string, actorID int64) error
	CountUnread(ctx context.Context, userID int64) (int, error)
	DeleteReadByUserID(ctx context.Context, userID int64) error
	DeleteReadByActor(ctx context.Context, userID int64, notifType string, actorID int64) error
	DeleteByIDs(ctx context.Context, ids []int64, userID int64) error
}
