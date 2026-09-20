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
	// CountAdministrators は管理者の総数だけを返す。初期セットアップ判定（0人か）が
	// 件数そのものを見るため、Exists ではなく Count を持たせている。
	// 削除の「最後の1人か」判定はロックが要るので CountAdministratorsForUpdate を使うこと。
	CountAdministrators(ctx context.Context) (int, error)
	// CountAdministratorsForUpdate は CountAdministrators と同じ件数を、行ロックを
	// 取ったうえで返す。「最後の1人は削除させない」判定と DELETE の間に別の削除が
	// 割り込めないようにするためのもので、必ずトランザクションの中から呼ぶこと
	// （トランザクション外では MySQL がロックを即座に手放すので意味が無い）。
	CountAdministratorsForUpdate(ctx context.Context) (int, error)
	UpdateAdministrator(ctx context.Context, a *model.Administrator) error
	SearchAdministratorsByName(ctx context.Context, name string) ([]*model.Administrator, error)
}
