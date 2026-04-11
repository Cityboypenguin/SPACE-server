package user

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type LogoutUserUseCase interface {
	Execute(ctx context.Context, token string) error
}

var _ LogoutUserUseCase = &LogoutUserInteractor{}

type LogoutUserInteractor struct {
	revokedTokenRepo repository.RevokedTokenRepository
}

func NewLogoutUserUseCase(revokedTokenRepo repository.RevokedTokenRepository) LogoutUserUseCase {
	return &LogoutUserInteractor{revokedTokenRepo: revokedTokenRepo}
}

func (uc *LogoutUserInteractor) Execute(ctx context.Context, token string) error {
	claims, err := auth.ValidateToken(token)
	if err != nil {
		return errors.New("invalid token")
	}

	return uc.revokedTokenRepo.RevokeToken(ctx, token, claims.ExpiresAt.Unix())
}
