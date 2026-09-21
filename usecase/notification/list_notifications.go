package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

const defaultLimit = 30

type ListNotificationsUseCase interface {
	Execute(ctx context.Context, q repository.PageQuery) ([]*model.Notification, int, error)
}

var _ ListNotificationsUseCase = &listNotificationsInteractor{}

type listNotificationsInteractor struct {
	repo repository.NotificationReader
}

func NewListNotificationsUseCase(repo repository.NotificationReader) ListNotificationsUseCase {
	return &listNotificationsInteractor{repo: repo}
}

func (uc *listNotificationsInteractor) Execute(ctx context.Context, q repository.PageQuery) ([]*model.Notification, int, error) {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return nil, 0, err
	}
	if q.Limit <= 0 {
		q.Limit = defaultLimit
	}
	return uc.repo.ListByUserID(ctx, userID, q)
}
