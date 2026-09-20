package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// GetUserByIDUseCase は表示用の1件取得。連絡先は返さない。
// トークン検証・凍結/解凍・プロフィール表示など、呼び出し元が本人とは限らない
// 経路はすべてこちらを使う。
type GetUserByIDUseCase interface {
	Execute(ctx context.Context, id int64) (*model.User, error)
}

var _ GetUserByIDUseCase = &GetUserByIDInteractor{}

type GetUserByIDInteractor struct {
	userRepo repository.UserRepository
}

func NewGetUserByIDUseCase(userRepo repository.UserRepository) GetUserByIDUseCase {
	return &GetUserByIDInteractor{
		userRepo: userRepo,
	}
}

func (uc *GetUserByIDInteractor) Execute(ctx context.Context, id int64) (*model.User, error) {
	return uc.userRepo.GetUserByID(ctx, id)
}

// GetUserAccountByIDUseCase は本人（me）・管理者（getUserByID）向けの1件取得。
// 連絡先を含むので、呼び出し元が本人か管理者であることを確かめた場所からだけ呼ぶこと。
//
// 認可をここに畳み込んでいないのは、「本人」と「管理者」で確かめ方が違うため
// （前者は claims.ID と引数の一致、後者は役割）。両方を引数で切り替える形にすると
// 呼び出し側の意図が読めなくなるので、判定はリゾルバに置いてある。
type GetUserAccountByIDUseCase interface {
	Execute(ctx context.Context, id int64) (*model.UserAccount, error)
}

var _ GetUserAccountByIDUseCase = &GetUserAccountByIDInteractor{}

type GetUserAccountByIDInteractor struct {
	userRepo repository.UserRepository
}

func NewGetUserAccountByIDUseCase(userRepo repository.UserRepository) GetUserAccountByIDUseCase {
	return &GetUserAccountByIDInteractor{userRepo: userRepo}
}

func (uc *GetUserAccountByIDInteractor) Execute(ctx context.Context, id int64) (*model.UserAccount, error) {
	return uc.userRepo.GetUserAccountByID(ctx, id)
}
