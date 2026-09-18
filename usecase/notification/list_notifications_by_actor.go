package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListNotificationsByActorUseCase interface {
	Execute(ctx context.Context, userID int64, notifType string, actorID int64, q repository.PageQuery) ([]*model.Notification, int, error)
}

var _ ListNotificationsByActorUseCase = &listNotificationsByActorInteractor{}

type listNotificationsByActorInteractor struct {
	repo repository.NotificationRepository
}

func NewListNotificationsByActorUseCase(repo repository.NotificationRepository) ListNotificationsByActorUseCase {
	return &listNotificationsByActorInteractor{repo: repo}
}

func (uc *listNotificationsByActorInteractor) Execute(ctx context.Context, userID int64, notifType string, actorID int64, q repository.PageQuery) ([]*model.Notification, int, error) {
	if q.Limit <= 0 {
		q.Limit = defaultLimit
	}
	return uc.repo.ListByActor(ctx, userID, notifType, actorID, q)
}
