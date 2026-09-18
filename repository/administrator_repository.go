package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type AdministratorRepository interface {
	SaveAdministrator(ctx context.Context, a *model.Administrator) error
	GetAdministratorByID(ctx context.Context, id int64) (*model.Administrator, error)
	FindByEmail(ctx context.Context, email string) (*model.Administrator, error)
	DeleteAdministrator(ctx context.Context, id int64) (bool, error)
	ListAdministrators(ctx context.Context, q PageQuery) ([]*model.Administrator, int, error)
	// CountAdministrators は管理者の総数だけを返す。初期セットアップ判定（0人か）と
	// 「最後の1人は削除させない」判定（1人以下か）の両方が件数そのものを見るため、
	// Exists ではなく Count を持たせている。
	CountAdministrators(ctx context.Context) (int, error)
	UpdateAdministrator(ctx context.Context, a *model.Administrator) error
	SearchAdministratorsByName(ctx context.Context, name string) ([]*model.Administrator, error)
}
