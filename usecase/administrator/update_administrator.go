package administrator

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdateAdministratorUseCase interface {
	Execute(ctx context.Context, id int64, param model.UpdateAdministratorParam) (*model.Administrator, error)
}

var _ UpdateAdministratorUseCase = &UpdateAdministratorInteractor{}

type UpdateAdministratorInteractor struct {
	adminRepo repository.AdministratorRepository
}

func NewUpdateAdministratorUseCase(adminRepo repository.AdministratorRepository) UpdateAdministratorUseCase {
	return &UpdateAdministratorInteractor{
		adminRepo: adminRepo,
	}
}

func (uc *UpdateAdministratorInteractor) Execute(ctx context.Context, id int64, param model.UpdateAdministratorParam) (*model.Administrator, error) {
	admin, err := uc.adminRepo.GetAdministratorByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if admin == nil {
		return nil, nil
	}

	err = admin.UpdateAdministrator(param)
	if err != nil {
		return nil, err
	}

	err = uc.adminRepo.SaveAdministrator(ctx, admin)
	if err != nil {
		return nil, err
	}

	return admin, nil
}
