package repository

import "context"

type Mailer interface {
	SendPasswordResetOTP(ctx context.Context, toEmail, otp string) error
}
