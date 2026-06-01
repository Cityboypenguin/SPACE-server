package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

const defaultLimit = 30

type ListNotificationsUseCase interface {
	Execute(ctx context.Context, userID int64, limit int) ([]*model.Notification, error)
}

var _ ListNotificationsUseCase = &listNotificationsInteractor{}

type listNotificationsInteractor struct {
	repo repository.NotificationRepository
}

func NewListNotificationsUseCase(repo repository.NotificationRepository) ListNotificationsUseCase {
	return &listNotificationsInteractor{repo: repo}
}

func (uc *listNotificationsInteractor) Execute(ctx context.Context, userID int64, limit int) ([]*model.Notification, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	return uc.repo.ListByUserID(ctx, userID, limit)
}
