package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

const defaultLimit = 30

type ListNotificationsUseCase interface {
	Execute(ctx context.Context, userID int64, q repository.PageQuery) ([]*model.Notification, int, error)
}

var _ ListNotificationsUseCase = &listNotificationsInteractor{}

type listNotificationsInteractor struct {
	repo repository.NotificationRepository
}

func NewListNotificationsUseCase(repo repository.NotificationRepository) ListNotificationsUseCase {
	return &listNotificationsInteractor{repo: repo}
}

func (uc *listNotificationsInteractor) Execute(ctx context.Context, userID int64, q repository.PageQuery) ([]*model.Notification, int, error) {
	if q.Limit <= 0 {
		q.Limit = defaultLimit
	}
	return uc.repo.ListByUserID(ctx, userID, q)
}
