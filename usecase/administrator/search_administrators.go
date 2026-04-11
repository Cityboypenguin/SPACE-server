package administrator

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SearchAdministratorsUseCase interface {
	Execute(ctx context.Context, name string) ([]*model.Administrator, error)
}

var _ SearchAdministratorsUseCase = &SearchAdministratorsInteractor{}

type SearchAdministratorsInteractor struct {
	adminRepo repository.AdministratorRepository
}

func NewSearchAdministratorsUseCase(adminRepo repository.AdministratorRepository) SearchAdministratorsUseCase {
	return &SearchAdministratorsInteractor{
		adminRepo: adminRepo,
	}
}

func (uc *SearchAdministratorsInteractor) Execute(ctx context.Context, name string) ([]*model.Administrator, error) {
	admins, err := uc.adminRepo.SearchAdministratorsByName(ctx, name)
	if err != nil {
		return nil, err
	}

	return admins, nil
}
