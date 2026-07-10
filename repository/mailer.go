package repository

import "context"

type Mailer interface {
	SendPasswordResetLink(ctx context.Context, toEmail, resetLink string) error
}
