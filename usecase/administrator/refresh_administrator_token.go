package administrator

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type RefreshAdministratorTokenResult struct {
	AccessToken   string
	RefreshToken  string
	Administrator *model.Administrator
}

type RefreshAdministratorTokenUseCase interface {
	Execute(ctx context.Context, refreshToken string) (*RefreshAdministratorTokenResult, error)
}

var _ RefreshAdministratorTokenUseCase = &RefreshAdministratorTokenInteractor{}

type RefreshAdministratorTokenInteractor struct {
	administratorRepo repository.AdministratorRepository
	revokedTokenRepo  repository.RevokedTokenRepository
}

func NewRefreshAdministratorTokenUseCase(
	administratorRepo repository.AdministratorRepository,
	revokedTokenRepo repository.RevokedTokenRepository,
) RefreshAdministratorTokenUseCase {
	return &RefreshAdministratorTokenInteractor{administratorRepo: administratorRepo, revokedTokenRepo: revokedTokenRepo}
}

func (uc *RefreshAdministratorTokenInteractor) Execute(ctx context.Context, refreshToken string) (*RefreshAdministratorTokenResult, error) {
	claims, err := auth.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}

	revoked, err := uc.revokedTokenRepo.IsRevoked(ctx, refreshToken)
	if err != nil {
		return nil, err
	}
	if revoked {
		return nil, errors.New("refresh token has been revoked")
	}

	if err := uc.revokedTokenRepo.RevokeToken(ctx, refreshToken, claims.ExpiresAt.Unix()); err != nil {
		return nil, err
	}

	accessToken, err := auth.GenerateAccessToken(claims.ID, claims.Role)
	if err != nil {
		return nil, err
	}
	newRefreshToken, err := auth.GenerateRefreshToken(claims.ID, claims.Role)
	if err != nil {
		return nil, err
	}

	a, err := uc.administratorRepo.GetAdministratorByID(ctx, claims.ID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, errors.New("administrator not found")
	}

	return &RefreshAdministratorTokenResult{AccessToken: accessToken, RefreshToken: newRefreshToken, Administrator: a}, nil
}
