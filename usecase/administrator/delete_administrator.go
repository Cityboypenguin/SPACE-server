package administrator

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteAdministratorUseCase interface {
	Execute(ctx context.Context, id int64) (bool, error)
}

var _ DeleteAdministratorUseCase = &DeleteAdministratorInteractor{}

type DeleteAdministratorInteractor struct {
	adminRepo repository.AdministratorRepository
}

func NewDeleteAdministratorUseCase(adminRepo repository.AdministratorRepository) DeleteAdministratorUseCase {
	return &DeleteAdministratorInteractor{
		adminRepo: adminRepo,
	}
}

func (uc *DeleteAdministratorInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	return uc.adminRepo.DeleteAdministrator(ctx, id)
}
