package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// SearchUsersUseCase は一般ユーザーのユーザー検索。誰でも（認証さえ通っていれば）
// 呼べるので、連絡先を含まない model.User を返す。
//
// 以前はここが model.User（当時は Email 持ち）を返し、GraphQL の User.email 経由で
// 他人のメールアドレスが取れていた。管理画面の検索は SearchUserAccountsUseCase。
type SearchUsersUseCase interface {
	Execute(ctx context.Context, keyword string, q repository.PageQuery) ([]*model.User, int, error)
}

var _ SearchUsersUseCase = &SearchUsersInteractor{}

type SearchUsersInteractor struct {
	userRepo repository.UserRepository
}

func NewSearchUsersUseCase(userRepo repository.UserRepository) SearchUsersUseCase {
	return &SearchUsersInteractor{
		userRepo: userRepo,
	}
}

func (uc *SearchUsersInteractor) Execute(ctx context.Context, keyword string, q repository.PageQuery) ([]*model.User, int, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, 0, err
	}

	users, total, err := uc.userRepo.SearchUsersByKeyword(ctx, keyword, q)
	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// SearchUserAccountsUseCase は管理画面のユーザー検索。連絡先を含むので管理者専用。
// 検索条件・並び順は SearchUsersUseCase と同じ（違うのは返す列だけ）。
type SearchUserAccountsUseCase interface {
	Execute(ctx context.Context, keyword string, q repository.PageQuery) ([]*model.UserAccount, int, error)
}

var _ SearchUserAccountsUseCase = &SearchUserAccountsInteractor{}

type SearchUserAccountsInteractor struct {
	userRepo repository.UserRepository
}

func NewSearchUserAccountsUseCase(userRepo repository.UserRepository) SearchUserAccountsUseCase {
	return &SearchUserAccountsInteractor{userRepo: userRepo}
}

func (uc *SearchUserAccountsInteractor) Execute(ctx context.Context, keyword string, q repository.PageQuery) ([]*model.UserAccount, int, error) {
	// ListUsersUseCase と同じく、連絡先を返す以上ユースケース単体でも管理者を確かめる。
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}
	return uc.userRepo.SearchUserAccountsByKeyword(ctx, keyword, q)
}
