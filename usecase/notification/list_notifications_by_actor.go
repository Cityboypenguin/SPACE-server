package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListNotificationsByActorUseCase interface {
	Execute(ctx context.Context, notifType string, actorID int64, q repository.PageQuery) ([]*model.Notification, int, error)
}

var _ ListNotificationsByActorUseCase = &listNotificationsByActorInteractor{}

type listNotificationsByActorInteractor struct {
	repo repository.NotificationReader
}

func NewListNotificationsByActorUseCase(repo repository.NotificationReader) ListNotificationsByActorUseCase {
	return &listNotificationsByActorInteractor{repo: repo}
}

func (uc *listNotificationsByActorInteractor) Execute(ctx context.Context, notifType string, actorID int64, q repository.PageQuery) ([]*model.Notification, int, error) {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return nil, 0, err
	}
	if q.Limit <= 0 {
		q.Limit = defaultLimit
	}
	return uc.repo.ListByActor(ctx, userID, notifType, actorID, q)
}
