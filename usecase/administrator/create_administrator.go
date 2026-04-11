package administrator

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreateAdministratorUseCase interface {
	Execute(ctx context.Context, param model.CreateAdministratorParam) (*model.Administrator, error)
}

var _ CreateAdministratorUseCase = &CreateAdministratorInteractor{}

type CreateAdministratorInteractor struct {
	adminRepo repository.AdministratorRepository
}

func NewCreateAdministratorUseCase(adminRepo repository.AdministratorRepository) CreateAdministratorUseCase {
	return &CreateAdministratorInteractor{
		adminRepo: adminRepo,
	}
}

func (uc *CreateAdministratorInteractor) Execute(ctx context.Context, param model.CreateAdministratorParam) (*model.Administrator, error) {
	admin := &model.Administrator{
		Name:           param.Name,
		Email:          param.Email,
		HashedPassword: param.Password,
	}
	err := uc.adminRepo.SaveAdministrator(ctx, admin)
	if err != nil {
		return nil, err
	}
	return admin, nil
}
