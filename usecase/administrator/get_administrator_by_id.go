package administrator

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetAdministratorByIDUseCase interface {
	Execute(ctx context.Context, id int64) (*model.Administrator, error)
}

var _ GetAdministratorByIDUseCase = &GetAdministratorByIDInteractor{}

type GetAdministratorByIDInteractor struct {
	adminRepo repository.AdministratorRepository
}

func NewGetAdministratorByIDUseCase(adminRepo repository.AdministratorRepository) GetAdministratorByIDUseCase {
	return &GetAdministratorByIDInteractor{
		adminRepo: adminRepo,
	}
}

func (uc *GetAdministratorByIDInteractor) Execute(ctx context.Context, id int64) (*model.Administrator, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	return uc.adminRepo.GetAdministratorByID(ctx, id)
}
