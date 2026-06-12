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
	_, total, err := uc.adminRepo.ListAdministrators(ctx, 1, 0)
	if err != nil {
		return 0, err
	}
	return total, nil
}
