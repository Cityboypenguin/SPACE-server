package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteNotificationsUseCase interface {
	Execute(ctx context.Context, ids []int64) error
}

var _ DeleteNotificationsUseCase = &deleteNotificationsInteractor{}

type deleteNotificationsInteractor struct {
	repo repository.NotificationDeleter
}

func NewDeleteNotificationsUseCase(repo repository.NotificationDeleter) DeleteNotificationsUseCase {
	return &deleteNotificationsInteractor{repo: repo}
}

func (uc *deleteNotificationsInteractor) Execute(ctx context.Context, ids []int64) error {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return err
	}
	return uc.repo.DeleteByIDs(ctx, ids, userID)
}
