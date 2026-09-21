package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetNotificationUseCase interface {
	Execute(ctx context.Context, id int64) (*model.Notification, error)
}

var _ GetNotificationUseCase = &getNotificationInteractor{}

type getNotificationInteractor struct {
	repo repository.NotificationReader
}

func NewGetNotificationUseCase(repo repository.NotificationReader) GetNotificationUseCase {
	return &getNotificationInteractor{repo: repo}
}

func (uc *getNotificationInteractor) Execute(ctx context.Context, id int64) (*model.Notification, error) {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return nil, err
	}
	return uc.repo.GetByID(ctx, id, userID)
}
