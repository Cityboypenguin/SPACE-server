package repository

import "context"

type PageViewInput struct {
	Path            string
	DurationSeconds int
	MaxScrollDepth  int
	// Resumed はタブへ戻って同じ画面の続きを見たぶん。閲覧数には数えない。
	Resumed bool
}

type SessionRepository interface {
	RecordSession(ctx context.Context, userID int64, durationSeconds int, pageViews []PageViewInput) error
}
