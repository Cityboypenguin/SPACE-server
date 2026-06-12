package repository

import (
	"context"
	"time"
)

type PasswordResetRepository interface {
	SaveOTP(ctx context.Context, email, otp string, ttl time.Duration) error
	GetOTP(ctx context.Context, email string) (string, error)
	DeleteOTP(ctx context.Context, email string) error

	SaveResetToken(ctx context.Context, token, email string, ttl time.Duration) error
	GetEmailByResetToken(ctx context.Context, token string) (string, error)
	DeleteResetToken(ctx context.Context, token string) error

	SetPasswordChangedAt(ctx context.Context, userID int64, t time.Time, ttl time.Duration) error
	GetPasswordChangedAt(ctx context.Context, userID int64) (*time.Time, error)
}
