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
	// User はトークンを更新した本人。ログインと同じく連絡先を含む。
	User *model.UserAccount
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

	if claims.Role == "administrator" || claims.CredentialsVersion == nil {
		return nil, errors.New("invalid refresh token")
	}

	// 返すのは本人ぶんなので連絡先込みで引く（UserAuthPayload.user は UserAccount）。
	u, err := uc.userRepo.GetUserAccountByID(ctx, claims.ID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, errors.New("user not found")
	}
	if u.Status == model.UserStatusFrozen {
		return nil, errors.New("account is frozen")
	}
	version, err := uc.userRepo.GetCredentialsVersionByID(ctx, claims.ID)
	if err != nil {
		return nil, err
	}
	if version != *claims.CredentialsVersion {
		return nil, errors.New("refresh token has been revoked")
	}
	accessToken, err := auth.GenerateUserAccessToken(claims.ID, claims.Role, version)
	if err != nil {
		return nil, err
	}
	newRefreshToken, err := auth.GenerateUserRefreshToken(claims.ID, claims.Role, version)
	if err != nil {
		return nil, err
	}

	consumed, err := uc.revokedTokenRepo.ConsumeToken(ctx, refreshToken, claims.ExpiresAt.Unix())
	if err != nil {
		return nil, err
	}
	if !consumed {
		return nil, errors.New("refresh token has been revoked")
	}

	return &RefreshUserTokenResult{AccessToken: accessToken, RefreshToken: newRefreshToken, User: u}, nil
}
