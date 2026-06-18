package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListNotificationGroupsUseCase interface {
	Execute(ctx context.Context, userID int64, limit, offset int) ([]*model.NotificationGroup, int, error)
}

var _ ListNotificationGroupsUseCase = &listNotificationGroupsInteractor{}

type listNotificationGroupsInteractor struct {
	repo repository.NotificationRepository
}

func NewListNotificationGroupsUseCase(repo repository.NotificationRepository) ListNotificationGroupsUseCase {
	return &listNotificationGroupsInteractor{repo: repo}
}

func (uc *listNotificationGroupsInteractor) Execute(ctx context.Context, userID int64, limit, offset int) ([]*model.NotificationGroup, int, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	return uc.repo.ListGroupedByUserID(ctx, userID, limit, offset)
}
