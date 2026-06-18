package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetNotificationUseCase interface {
	Execute(ctx context.Context, id int64, userID int64) (*model.Notification, error)
}

var _ GetNotificationUseCase = &getNotificationInteractor{}

type getNotificationInteractor struct {
	repo repository.NotificationRepository
}

func NewGetNotificationUseCase(repo repository.NotificationRepository) GetNotificationUseCase {
	return &getNotificationInteractor{repo: repo}
}

func (uc *getNotificationInteractor) Execute(ctx context.Context, id int64, userID int64) (*model.Notification, error) {
	return uc.repo.GetByID(ctx, id, userID)
}
