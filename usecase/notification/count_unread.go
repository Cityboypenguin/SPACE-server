package notification

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CountUnreadUseCase interface {
	Execute(ctx context.Context) (int, error)
}

var _ CountUnreadUseCase = &countUnreadInteractor{}

type countUnreadInteractor struct {
	repo repository.NotificationReader
}

func NewCountUnreadUseCase(repo repository.NotificationReader) CountUnreadUseCase {
	return &countUnreadInteractor{repo: repo}
}

func (uc *countUnreadInteractor) Execute(ctx context.Context) (int, error) {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return 0, err
	}
	return uc.repo.CountUnread(ctx, userID)
}
