package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListNotificationGroupsUseCase interface {
	Execute(ctx context.Context, userID int64, q repository.PageQuery) ([]*model.NotificationGroup, int, error)
}

var _ ListNotificationGroupsUseCase = &listNotificationGroupsInteractor{}

type listNotificationGroupsInteractor struct {
	repo repository.NotificationRepository
}

func NewListNotificationGroupsUseCase(repo repository.NotificationRepository) ListNotificationGroupsUseCase {
	return &listNotificationGroupsInteractor{repo: repo}
}

func (uc *listNotificationGroupsInteractor) Execute(ctx context.Context, userID int64, q repository.PageQuery) ([]*model.NotificationGroup, int, error) {
	if q.Limit <= 0 {
		q.Limit = defaultLimit
	}
	return uc.repo.ListGroupedByUserID(ctx, userID, q)
}
