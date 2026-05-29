package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CountUnreadUseCase interface {
	Execute(ctx context.Context, userID int64) (int, error)
}

var _ CountUnreadUseCase = &countUnreadInteractor{}

type countUnreadInteractor struct {
	repo repository.NotificationRepository
}

func NewCountUnreadUseCase(repo repository.NotificationRepository) CountUnreadUseCase {
	return &countUnreadInteractor{repo: repo}
}

func (uc *countUnreadInteractor) Execute(ctx context.Context, userID int64) (int, error) {
	return uc.repo.CountUnread(ctx, userID)
}
