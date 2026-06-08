package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type EmailOTPRepository interface {
	Save(ctx context.Context, otp *model.EmailOTP) error
	FindLatestByEmail(ctx context.Context, email string) (*model.EmailOTP, error)
	Delete(ctx context.Context, email string) error
	IsRateLimited(ctx context.Context, email string) (bool, error)
	MarkRateLimited(ctx context.Context, email string) error
}
