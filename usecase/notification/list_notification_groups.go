package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListNotificationGroupsUseCase interface {
	Execute(ctx context.Context, q repository.PageQuery) ([]*model.NotificationGroup, int, error)
}

var _ ListNotificationGroupsUseCase = &listNotificationGroupsInteractor{}

type listNotificationGroupsInteractor struct {
	repo repository.NotificationReader
}

func NewListNotificationGroupsUseCase(repo repository.NotificationReader) ListNotificationGroupsUseCase {
	return &listNotificationGroupsInteractor{repo: repo}
}

func (uc *listNotificationGroupsInteractor) Execute(ctx context.Context, q repository.PageQuery) ([]*model.NotificationGroup, int, error) {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return nil, 0, err
	}
	if q.Limit <= 0 {
		q.Limit = defaultLimit
	}
	return uc.repo.ListGroupedByUserID(ctx, userID, q)
}
