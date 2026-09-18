package user

import (
	"context"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"golang.org/x/crypto/bcrypt"
)

type UpdateUserUseCase interface {
	Execute(ctx context.Context, id int64, param model.UpdateUserParam, currentPassword *string, requireCurrentPassword bool) (*model.UserAccount, error)
}

var _ UpdateUserUseCase = &UpdateUserInteractor{}

type UpdateUserInteractor struct {
	userRepo repository.UserRepository
}

func NewUpdateUserUseCase(userRepo repository.UserRepository) UpdateUserUseCase {
	return &UpdateUserInteractor{
		userRepo: userRepo,
	}
}

func (uc *UpdateUserInteractor) Execute(ctx context.Context, id int64, param model.UpdateUserParam, currentPassword *string, requireCurrentPassword bool) (*model.UserAccount, error) {
	// このユースケースはパスワードを変えられる（param.Password）ので、
	// 現在パスワードの照合と保存の両方でハッシュが要る。
	user, err := uc.userRepo.GetCredentialsByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	if requireCurrentPassword && param.Password != nil {
		if currentPassword == nil || *currentPassword == "" {
			return nil, fmt.Errorf("現在のパスワードを入力してください")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.HashedPassword), []byte(*currentPassword)); err != nil {
			return nil, fmt.Errorf("現在のパスワードが正しくありません")
		}
	}

	err = user.UpdateUser(param)
	if err != nil {
		return nil, err
	}

	err = uc.userRepo.SaveCredentials(ctx, user)
	if err != nil {
		return nil, err
	}

	// 返すのはパスワードハッシュを外した本人ぶん（リゾルバは toGraphUserAccount に渡す）。
	// 呼び出し元は本人（updateUser）か管理者（adminUpdateUser）に限られるので、
	// 連絡先を含めてよい。ハッシュだけは絶対に外に出さないので、ここで切り落とす。
	account := user.UserAccount
	return &account, nil
}
