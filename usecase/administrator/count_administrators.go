package administrator

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CountAdministratorsUseCase interface {
	Execute(ctx context.Context) (int, error)
}

var _ CountAdministratorsUseCase = &CountAdministratorsInteractor{}

type CountAdministratorsInteractor struct {
	adminRepo repository.AdministratorRepository
}

func NewCountAdministratorsUseCase(adminRepo repository.AdministratorRepository) CountAdministratorsUseCase {
	return &CountAdministratorsInteractor{adminRepo: adminRepo}
}

func (uc *CountAdministratorsInteractor) Execute(ctx context.Context) (int, error) {
	return uc.adminRepo.CountAdministrators(ctx)
}
