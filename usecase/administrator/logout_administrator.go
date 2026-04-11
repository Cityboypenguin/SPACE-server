package administrator

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type LogoutAdministratorUseCase interface {
	Execute(ctx context.Context, token string) error
}

var _ LogoutAdministratorUseCase = &LogoutAdministratorInteractor{}

type LogoutAdministratorInteractor struct {
	revokedTokenRepo repository.RevokedTokenRepository
}

func NewLogoutAdministratorUseCase(revokedTokenRepo repository.RevokedTokenRepository) LogoutAdministratorUseCase {
	return &LogoutAdministratorInteractor{revokedTokenRepo: revokedTokenRepo}
}

func (uc *LogoutAdministratorInteractor) Execute(ctx context.Context, token string) error {
	claims, err := auth.ValidateToken(token)
	if err != nil {
		return errors.New("invalid token")
	}

	return uc.revokedTokenRepo.RevokeToken(ctx, token, claims.ExpiresAt.Unix())
}
