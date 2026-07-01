package session

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type RecordSessionUseCase interface {
	Execute(ctx context.Context, durationSeconds int, pageViews []repository.PageViewInput) error
}

var _ RecordSessionUseCase = &RecordSessionInteractor{}

type RecordSessionInteractor struct {
	sessionRepo repository.SessionRepository
}

func NewRecordSessionUseCase(sessionRepo repository.SessionRepository) RecordSessionUseCase {
	return &RecordSessionInteractor{sessionRepo: sessionRepo}
}

func (uc *RecordSessionInteractor) Execute(ctx context.Context, durationSeconds int, pageViews []repository.PageViewInput) error {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil // 未認証ユーザーは記録しない
	}
	return uc.sessionRepo.RecordSession(ctx, claims.ID, durationSeconds, pageViews)
}
