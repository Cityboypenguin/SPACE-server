package administrator

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteAdministratorUseCase interface {
	Execute(ctx context.Context, id int64) (bool, error)
}

var _ DeleteAdministratorUseCase = &DeleteAdministratorInteractor{}

type DeleteAdministratorInteractor struct {
	adminRepo repository.AdministratorRepository
	txManager repository.TxManager
}

func NewDeleteAdministratorUseCase(adminRepo repository.AdministratorRepository, txManager repository.TxManager) DeleteAdministratorUseCase {
	return &DeleteAdministratorInteractor{
		adminRepo: adminRepo,
		txManager: txManager,
	}
}

// Execute は管理者を1人削除する。ただし最後の1人は残す。
//
// 数えるのと消すのを1つのトランザクションに入れ、数えるほうで行ロックを取るのが要点。
// 以前は COUNT と DELETE が別々の接続・別々のトランザクションで走っていたため、
// 管理者が2人のときに2つの削除要求が同時に来ると、両方が COUNT=2 を読んでから
// 両方が DELETE でき、管理者が0人になった（そうなると管理画面に誰も入れない）。
// いまは後から来たほうがロックを待ち、相手のコミット後に COUNT=1 を読むので弾かれる。
func (uc *DeleteAdministratorInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return false, err
	}
	var deleted bool
	err := uc.txManager.RunInTx(ctx, func(txCtx context.Context) error {
		// 件数だけが要るので、行を1件取って捨てる ListAdministrators ではなく Count を使う。
		// 同時削除を直列化するため、ロックを取る版を使う。
		total, err := uc.adminRepo.CountAdministratorsForUpdate(txCtx)
		if err != nil {
			return err
		}
		if total <= 1 {
			return errors.New("最後の管理者は削除できません")
		}
		deleted, err = uc.adminRepo.DeleteAdministrator(txCtx, id)
		return err
	})
	if err != nil {
		return false, err
	}
	return deleted, nil
}
