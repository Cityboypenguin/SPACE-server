package usecase

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/domain"
	"github.com/Cityboypenguin/SPACE-server/graph/model"
)

type userUsecase struct {
	repo domain.UserRepository
}

func NewUserUsecase(repo domain.UserRepository) domain.UserUsecase {
	return &userUsecase{repo: repo}
}

// Create
func (u *userUsecase) CreateUser(ctx context.Context, name string, email string) (*model.User, error) {
	// 本当はここでID採番のためにRepoのCountなどを呼ぶべきですが簡略化のため固定文字+ランダム等推奨
	// 今回はRepoのメモリ数が見えないので簡易的に作成
	user := &model.User{ID: "U_TEMP", Name: name, Email: email}
	// ※注意: 正確なID採番(len+1)をするにはRepoにCountメソッドが必要ですが、
	// いったん「UUID」などを使うか、Repo側でID採番する形にするのが一般的です。
	// 今回は学習用なのでこのままでOK、またはRepoで採番するように修正もアリです。
	return u.repo.Create(ctx, user)
}

// UpdateName
func (u *userUsecase) UpdateName(ctx context.Context, id string, newName string) (*model.User, error) {
	user, err := u.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	user.Name = newName
	return u.repo.Update(ctx, user)
}

// UpdateEmail
func (u *userUsecase) UpdateEmail(ctx context.Context, id string, newEmail string) (*model.User, error) {
	user, err := u.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	user.Email = newEmail
	return u.repo.Update(ctx, user)
}

// Delete
func (u *userUsecase) DeleteAccount(ctx context.Context, id string) (bool, error) {
	return u.repo.Delete(ctx, id)
}

// GetOne
func (u *userUsecase) GetUser(ctx context.Context, id string) (*model.User, error) {
	return u.repo.GetByID(ctx, id)
}

// GetAll
func (u *userUsecase) GetUsers(ctx context.Context) ([]*model.User, error) {
	return u.repo.GetAll(ctx)
}
