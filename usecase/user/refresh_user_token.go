package user

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type RefreshUserTokenResult struct {
	AccessToken  string
	RefreshToken string
	User         *model.User
}

type RefreshUserTokenUseCase interface {
	Execute(ctx context.Context, refreshToken string) (*RefreshUserTokenResult, error)
}

var _ RefreshUserTokenUseCase = &RefreshUserTokenInteractor{}

type RefreshUserTokenInteractor struct {
	userRepo         repository.UserRepository
	revokedTokenRepo repository.RevokedTokenRepository
}

func NewRefreshUserTokenUseCase(
	userRepo repository.UserRepository,
	revokedTokenRepo repository.RevokedTokenRepository,
) RefreshUserTokenUseCase {
	return &RefreshUserTokenInteractor{userRepo: userRepo, revokedTokenRepo: revokedTokenRepo}
}

func (uc *RefreshUserTokenInteractor) Execute(ctx context.Context, refreshToken string) (*RefreshUserTokenResult, error) {
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

	accessToken, err := auth.GenerateAccessToken(claims.ID, claims.Role)
	if err != nil {
		return nil, err
	}
	newRefreshToken, err := auth.GenerateRefreshToken(claims.ID, claims.Role)
	if err != nil {
		return nil, err
	}

	u, err := uc.userRepo.GetUserByID(ctx, claims.ID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, errors.New("user not found")
	}

	if err := uc.revokedTokenRepo.RevokeToken(ctx, refreshToken, claims.ExpiresAt.Unix()); err != nil {
		return nil, err
	}

	return &RefreshUserTokenResult{AccessToken: accessToken, RefreshToken: newRefreshToken, User: u}, nil
}
