package repository

import (
	"context"
	"time"
)

type PasswordResetRepository interface {
	SaveOTP(ctx context.Context, email, otp string, ttl time.Duration) error
	ExchangeOTPForResetToken(ctx context.Context, email, otp, token string, ttl time.Duration) (bool, error)
	TryBeginRequest(ctx context.Context, email string, ttl time.Duration) (bool, error)

	ConsumeResetToken(ctx context.Context, token string) (string, error)
}
