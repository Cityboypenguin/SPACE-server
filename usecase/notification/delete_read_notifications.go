package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteReadNotificationsUseCase interface {
	Execute(ctx context.Context) error
}

var _ DeleteReadNotificationsUseCase = &deleteReadNotificationsInteractor{}

type deleteReadNotificationsInteractor struct {
	repo repository.NotificationDeleter
}

func NewDeleteReadNotificationsUseCase(repo repository.NotificationDeleter) DeleteReadNotificationsUseCase {
	return &deleteReadNotificationsInteractor{repo: repo}
}

func (uc *deleteReadNotificationsInteractor) Execute(ctx context.Context) error {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return err
	}
	return uc.repo.DeleteReadByUserID(ctx, userID)
}

type DeleteReadNotificationsByActorUseCase interface {
	Execute(ctx context.Context, notifType string, actorID int64) error
}

var _ DeleteReadNotificationsByActorUseCase = &deleteReadNotificationsByActorInteractor{}

type deleteReadNotificationsByActorInteractor struct {
	repo repository.NotificationDeleter
}

func NewDeleteReadNotificationsByActorUseCase(repo repository.NotificationDeleter) DeleteReadNotificationsByActorUseCase {
	return &deleteReadNotificationsByActorInteractor{repo: repo}
}

func (uc *deleteReadNotificationsByActorInteractor) Execute(ctx context.Context, notifType string, actorID int64) error {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return err
	}
	return uc.repo.DeleteReadByActor(ctx, userID, notifType, actorID)
}
