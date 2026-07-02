package repository

import "context"

type PageViewInput struct {
	Path            string
	DurationSeconds int
	MaxScrollDepth  int
}

type SessionRepository interface {
	RecordSession(ctx context.Context, userID int64, durationSeconds int, pageViews []PageViewInput) error
}
