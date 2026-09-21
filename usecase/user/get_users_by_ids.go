package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type GetUsersByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) ([]*model.User, error)
}

var _ GetUsersByIDsUseCase = &GetUsersByIDsInteractor{}

type GetUsersByIDsInteractor struct {
	userRepo userDirectoryRepository
}

func NewGetUsersByIDsUseCase(userRepo userDirectoryRepository) GetUsersByIDsUseCase {
	return &GetUsersByIDsInteractor{userRepo: userRepo}
}

func (uc *GetUsersByIDsInteractor) Execute(ctx context.Context, ids []int64) ([]*model.User, error) {
	return uc.userRepo.GetUsersByIDs(ctx, ids)
}

// GetUserAccountsByIDsUseCase はまとめて引く版の本人・管理者向け取得。
// 規約の同意者一覧（管理者専用）が同意レコードを人に解決するために使う。
// 表示のために引くだけなら GetUsersByIDsUseCase を使うこと。
type GetUserAccountsByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) ([]*model.UserAccount, error)
}

var _ GetUserAccountsByIDsUseCase = &GetUserAccountsByIDsInteractor{}

type GetUserAccountsByIDsInteractor struct {
	userRepo userDirectoryRepository
}

func NewGetUserAccountsByIDsUseCase(userRepo userDirectoryRepository) GetUserAccountsByIDsUseCase {
	return &GetUserAccountsByIDsInteractor{userRepo: userRepo}
}

func (uc *GetUserAccountsByIDsInteractor) Execute(ctx context.Context, ids []int64) ([]*model.UserAccount, error) {
	return uc.userRepo.GetUserAccountsByIDs(ctx, ids)
}
