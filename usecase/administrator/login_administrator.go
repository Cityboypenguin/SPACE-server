package administrator

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/loginguard"
	"golang.org/x/crypto/bcrypt"
)

type LoginAdministratorResult struct {
	AccessToken   string
	RefreshToken  string
	Administrator *model.Administrator
}

type LoginAdministratorUseCase interface {
	Execute(ctx context.Context, email, password string) (*LoginAdministratorResult, error)
}

var _ LoginAdministratorUseCase = &LoginAdministratorInteractor{}

type LoginAdministratorInteractor struct {
	adminRepo repository.AdministratorRepository
}

func NewLoginAdministratorUseCase(adminRepo repository.AdministratorRepository) LoginAdministratorUseCase {
	return &LoginAdministratorInteractor{adminRepo: adminRepo}
}

type guardedLoginAdministrator struct {
	inner LoginAdministratorUseCase
	guard *loginguard.Guard
}

// NewGuardedLoginAdministratorUseCase は inner を、失敗が続いたアカウントを一時的に止める形で包む。
func NewGuardedLoginAdministratorUseCase(inner LoginAdministratorUseCase, guard *loginguard.Guard) LoginAdministratorUseCase {
	return &guardedLoginAdministrator{inner: inner, guard: guard}
}

func (uc *guardedLoginAdministrator) Execute(ctx context.Context, email, password string) (*LoginAdministratorResult, error) {
	return loginguard.Attempt(ctx, uc.guard, loginguard.ScopeAdministrator, email, func() (*LoginAdministratorResult, error) {
		return uc.inner.Execute(ctx, email, password)
	})
}

func (uc *LoginAdministratorInteractor) Execute(ctx context.Context, email, password string) (*LoginAdministratorResult, error) {
	admin, err := uc.adminRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if admin == nil {
		loginguard.CompareWithoutAccount(password)
		return nil, loginguard.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.HashedPassword), []byte(password)); err != nil {
		return nil, loginguard.ErrInvalidCredentials
	}

	accessToken, err := auth.GenerateAccessToken(admin.ID, "administrator")
	if err != nil {
		return nil, err
	}

	refreshToken, err := auth.GenerateRefreshToken(admin.ID, "administrator")
	if err != nil {
		return nil, err
	}

	return &LoginAdministratorResult{AccessToken: accessToken, RefreshToken: refreshToken, Administrator: admin}, nil
}
