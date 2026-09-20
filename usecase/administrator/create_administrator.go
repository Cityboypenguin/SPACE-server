package administrator

import (
	"context"
	"errors"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreateAdministratorUseCase interface {
	Execute(ctx context.Context, param model.CreateAdministratorParam) (*model.Administrator, error)
}

var _ CreateAdministratorUseCase = &CreateAdministratorInteractor{}

type CreateAdministratorInteractor struct {
	adminRepo repository.AdministratorRepository
	txManager repository.TxManager
}

func NewCreateAdministratorUseCase(adminRepo repository.AdministratorRepository, txManager repository.TxManager) CreateAdministratorUseCase {
	return &CreateAdministratorInteractor{
		adminRepo: adminRepo,
		txManager: txManager,
	}
}

func (uc *CreateAdministratorInteractor) Execute(ctx context.Context, param model.CreateAdministratorParam) (*model.Administrator, error) {
	claims, authenticated := auth.ClaimsFromContext(ctx)
	if authenticated && !authz.IsAdminRole(claims.Role) {
		return nil, errors.New("forbidden")
	}
	admin := &model.Administrator{}
	now := time.Now()

	// Set timestamps explicitly
	param.CreatedAt = now
	param.UpdatedAt = now

	if err := admin.CreateAdministrator(param); err != nil {
		return nil, err
	}

	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		total, err := uc.adminRepo.CountAdministratorsForUpdate(ctx)
		if err != nil {
			return err
		}
		if !authenticated && total > 0 {
			return errors.New("unauthorized")
		}
		return uc.adminRepo.SaveAdministrator(ctx, admin)
	}); err != nil {
		return nil, err
	}
	return admin, nil
}
