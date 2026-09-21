package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MarkAsReadUseCase interface {
	Execute(ctx context.Context, id int64) error
}

var _ MarkAsReadUseCase = &markAsReadInteractor{}

type markAsReadInteractor struct {
	repo repository.NotificationReadStateRepository
}

func NewMarkAsReadUseCase(repo repository.NotificationReadStateRepository) MarkAsReadUseCase {
	return &markAsReadInteractor{repo: repo}
}

func (uc *markAsReadInteractor) Execute(ctx context.Context, id int64) error {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return err
	}
	return uc.repo.MarkAsRead(ctx, id, userID)
}

type MarkAllAsReadUseCase interface {
	Execute(ctx context.Context) error
}

var _ MarkAllAsReadUseCase = &markAllAsReadInteractor{}

type markAllAsReadInteractor struct {
	repo repository.NotificationReadStateRepository
}

func NewMarkAllAsReadUseCase(repo repository.NotificationReadStateRepository) MarkAllAsReadUseCase {
	return &markAllAsReadInteractor{repo: repo}
}

func (uc *markAllAsReadInteractor) Execute(ctx context.Context) error {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return err
	}
	return uc.repo.MarkAllAsRead(ctx, userID)
}

type MarkAllAsReadByActorUseCase interface {
	Execute(ctx context.Context, notifType string, actorID int64) error
}

var _ MarkAllAsReadByActorUseCase = &markAllAsReadByActorInteractor{}

type markAllAsReadByActorInteractor struct {
	repo repository.NotificationReadStateRepository
}

func NewMarkAllAsReadByActorUseCase(repo repository.NotificationReadStateRepository) MarkAllAsReadByActorUseCase {
	return &markAllAsReadByActorInteractor{repo: repo}
}

func (uc *markAllAsReadByActorInteractor) Execute(ctx context.Context, notifType string, actorID int64) error {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return err
	}
	return uc.repo.MarkAllAsReadByActor(ctx, userID, notifType, actorID)
}
