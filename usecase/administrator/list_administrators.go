package administrator

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListAdministratorsUseCase interface {
	Execute(ctx context.Context) ([]*model.Administrator, error)
}

var _ ListAdministratorsUseCase = &ListAdministratorsInteractor{}

type ListAdministratorsInteractor struct {
	adminRepo repository.AdministratorRepository
}

func NewListAdministratorsUseCase(adminRepo repository.AdministratorRepository) ListAdministratorsUseCase {
	return &ListAdministratorsInteractor{
		adminRepo: adminRepo,
	}
}

func (uc *ListAdministratorsInteractor) Execute(ctx context.Context) ([]*model.Administrator, error) {
	return uc.adminRepo.ListAdministrators(ctx)
}
