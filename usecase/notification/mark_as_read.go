package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MarkAsReadUseCase interface {
	Execute(ctx context.Context, id int64, userID int64) error
}

var _ MarkAsReadUseCase = &markAsReadInteractor{}

type markAsReadInteractor struct {
	repo repository.NotificationRepository
}

func NewMarkAsReadUseCase(repo repository.NotificationRepository) MarkAsReadUseCase {
	return &markAsReadInteractor{repo: repo}
}

func (uc *markAsReadInteractor) Execute(ctx context.Context, id int64, userID int64) error {
	return uc.repo.MarkAsRead(ctx, id, userID)
}

type MarkAllAsReadUseCase interface {
	Execute(ctx context.Context, userID int64) error
}

var _ MarkAllAsReadUseCase = &markAllAsReadInteractor{}

type markAllAsReadInteractor struct {
	repo repository.NotificationRepository
}

func NewMarkAllAsReadUseCase(repo repository.NotificationRepository) MarkAllAsReadUseCase {
	return &markAllAsReadInteractor{repo: repo}
}

func (uc *markAllAsReadInteractor) Execute(ctx context.Context, userID int64) error {
	return uc.repo.MarkAllAsRead(ctx, userID)
}

type MarkAllAsReadByActorUseCase interface {
	Execute(ctx context.Context, userID int64, notifType string, actorID int64) error
}

var _ MarkAllAsReadByActorUseCase = &markAllAsReadByActorInteractor{}

type markAllAsReadByActorInteractor struct {
	repo repository.NotificationRepository
}

func NewMarkAllAsReadByActorUseCase(repo repository.NotificationRepository) MarkAllAsReadByActorUseCase {
	return &markAllAsReadByActorInteractor{repo: repo}
}

func (uc *markAllAsReadByActorInteractor) Execute(ctx context.Context, userID int64, notifType string, actorID int64) error {
	return uc.repo.MarkAllAsReadByActor(ctx, userID, notifType, actorID)
}
