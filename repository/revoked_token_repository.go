package repository

import "context"

type RevokedTokenRepository interface {
	RevokeToken(ctx context.Context, token string, expiresAt int64) error
	IsRevoked(ctx context.Context, token string) (bool, error)
	ConsumeToken(ctx context.Context, token string, expiresAt int64) (bool, error)
}
