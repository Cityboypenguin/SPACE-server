package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type EmailOTPRepository interface {
	Save(ctx context.Context, otp *model.EmailOTP) error
	FindLatestByEmail(ctx context.Context, email string) (*model.EmailOTP, error)
	Delete(ctx context.Context, email string) error
	// RecordFailure は間違った入力を数え、続いたらコードを捨てる（6桁なので総当たりを止める）。
	RecordFailure(ctx context.Context, email string) error
	TryBeginSend(ctx context.Context, email string) (bool, error)
}
