package administrator

import (
	"context"
	"errors"

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
	_, total, err := uc.adminRepo.ListAdministrators(ctx, 1, 0)
	if err != nil {
		return false, err
	}
	if total <= 1 {
		return false, errors.New("最後の管理者は削除できません")
	}
	return uc.adminRepo.DeleteAdministrator(ctx, id)
}
