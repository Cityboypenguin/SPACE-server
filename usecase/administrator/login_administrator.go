package administrator

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"golang.org/x/crypto/bcrypt"
)

type LoginAdministratorResult struct {
	Token         string
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

func (uc *LoginAdministratorInteractor) Execute(ctx context.Context, email, password string) (*LoginAdministratorResult, error) {
	admin, err := uc.adminRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if admin == nil {
		return nil, errors.New("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.HashedPassword), []byte(password)); err != nil {
		return nil, errors.New("invalid email or password")
	}

	token, err := auth.GenerateToken(admin.ID, "administrator")
	if err != nil {
		return nil, err
	}

	return &LoginAdministratorResult{Token: token, Administrator: admin}, nil
}
