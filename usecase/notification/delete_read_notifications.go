package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteReadNotificationsUseCase interface {
	Execute(ctx context.Context, userID int64) error
}

var _ DeleteReadNotificationsUseCase = &deleteReadNotificationsInteractor{}

type deleteReadNotificationsInteractor struct {
	repo repository.NotificationRepository
}

func NewDeleteReadNotificationsUseCase(repo repository.NotificationRepository) DeleteReadNotificationsUseCase {
	return &deleteReadNotificationsInteractor{repo: repo}
}

func (uc *deleteReadNotificationsInteractor) Execute(ctx context.Context, userID int64) error {
	return uc.repo.DeleteReadByUserID(ctx, userID)
}

type DeleteReadNotificationsByActorUseCase interface {
	Execute(ctx context.Context, userID int64, notifType string, actorID int64) error
}

var _ DeleteReadNotificationsByActorUseCase = &deleteReadNotificationsByActorInteractor{}

type deleteReadNotificationsByActorInteractor struct {
	repo repository.NotificationRepository
}

func NewDeleteReadNotificationsByActorUseCase(repo repository.NotificationRepository) DeleteReadNotificationsByActorUseCase {
	return &deleteReadNotificationsByActorInteractor{repo: repo}
}

func (uc *deleteReadNotificationsByActorInteractor) Execute(ctx context.Context, userID int64, notifType string, actorID int64) error {
	return uc.repo.DeleteReadByActor(ctx, userID, notifType, actorID)
}
