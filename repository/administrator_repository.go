package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type AdministratorRepository interface {
	SaveAdministrator(ctx context.Context, a *model.Administrator) error
	GetAdministratorByID(ctx context.Context, id int64) (*model.Administrator, error)
	DeleteAdministrator(ctx context.Context, id int64) (bool, error)
	ListAdministrators(ctx context.Context) ([]*model.Administrator, error)
	UpdateAdministrator(ctx context.Context, a *model.Administrator) error
	SearchAdministratorsByName(ctx context.Context, name string) ([]*model.Administrator, error)
}
