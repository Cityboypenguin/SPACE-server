package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteNotificationsUseCase interface {
	Execute(ctx context.Context, ids []int64, userID int64) error
}

var _ DeleteNotificationsUseCase = &deleteNotificationsInteractor{}

type deleteNotificationsInteractor struct {
	repo repository.NotificationRepository
}

func NewDeleteNotificationsUseCase(repo repository.NotificationRepository) DeleteNotificationsUseCase {
	return &deleteNotificationsInteractor{repo: repo}
}

func (uc *deleteNotificationsInteractor) Execute(ctx context.Context, ids []int64, userID int64) error {
	return uc.repo.DeleteByIDs(ctx, ids, userID)
}
