package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ListUsersUseCase は管理画面のユーザー台帳。管理者しか呼べないので、
// 連絡先を含む model.UserAccount を返す（GraphQL の UserAccountPage に対応）。
type ListUsersUseCase interface {
	Execute(ctx context.Context, q repository.PageQuery) ([]*model.UserAccount, int, error)
}

var _ ListUsersUseCase = &ListUsersInteractor{}

type ListUsersInteractor struct {
	userRepo repository.UserAccountRepository
}

func NewListUsersUseCase(userRepo repository.UserAccountRepository) ListUsersUseCase {
	return &ListUsersInteractor{
		userRepo: userRepo,
	}
}

func (uc *ListUsersInteractor) Execute(ctx context.Context, q repository.PageQuery) ([]*model.UserAccount, int, error) {
	// 連絡先を返すので、ここで管理者であることを必ず確かめる。
	// リゾルバ側でも requireAdminAuth しているが、この型を返す以上
	// ユースケース単体でも成り立たせておく。
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}

	return uc.userRepo.ListUserAccounts(ctx, q)
}
