package administrator

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListAdministratorsUseCase interface {
	Execute(ctx context.Context, limit, offset int) ([]*model.Administrator, int, error)
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

func (uc *ListAdministratorsInteractor) Execute(ctx context.Context, limit, offset int) ([]*model.Administrator, int, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}

	return uc.adminRepo.ListAdministrators(ctx, limit, offset)
}
