package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetUsersByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) ([]*model.User, error)
}

var _ GetUsersByIDsUseCase = &GetUsersByIDsInteractor{}

type GetUsersByIDsInteractor struct {
	userRepo repository.UserRepository
}

func NewGetUsersByIDsUseCase(userRepo repository.UserRepository) GetUsersByIDsUseCase {
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
	userRepo repository.UserRepository
}

func NewGetUserAccountsByIDsUseCase(userRepo repository.UserRepository) GetUserAccountsByIDsUseCase {
	return &GetUserAccountsByIDsInteractor{userRepo: userRepo}
}

func (uc *GetUserAccountsByIDsInteractor) Execute(ctx context.Context, ids []int64) ([]*model.UserAccount, error) {
	return uc.userRepo.GetUserAccountsByIDs(ctx, ids)
}
