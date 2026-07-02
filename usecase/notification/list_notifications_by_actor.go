package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListNotificationsByActorUseCase interface {
	Execute(ctx context.Context, userID int64, notifType string, actorID int64, limit, offset int) ([]*model.Notification, int, error)
}

var _ ListNotificationsByActorUseCase = &listNotificationsByActorInteractor{}

type listNotificationsByActorInteractor struct {
	repo repository.NotificationRepository
}

func NewListNotificationsByActorUseCase(repo repository.NotificationRepository) ListNotificationsByActorUseCase {
	return &listNotificationsByActorInteractor{repo: repo}
}

func (uc *listNotificationsByActorInteractor) Execute(ctx context.Context, userID int64, notifType string, actorID int64, limit, offset int) ([]*model.Notification, int, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	return uc.repo.ListByActor(ctx, userID, notifType, actorID, limit, offset)
}
