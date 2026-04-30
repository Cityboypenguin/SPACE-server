package administrator

import (
	"context"
	"time"

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
	admin := &model.Administrator{}
	now := time.Now()

	// Set timestamps explicitly
	param.CreatedAt = now
	param.UpdatedAt = now

	if err := admin.CreateAdministrator(param); err != nil {
		return nil, err
	}

	if err := uc.adminRepo.SaveAdministrator(ctx, admin); err != nil {
		return nil, err
	}
	return admin, nil
}
