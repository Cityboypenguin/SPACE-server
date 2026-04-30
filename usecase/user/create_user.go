package user

import (
	"context"
	"time"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreateUserUseCase interface {
	Execute(ctx context.Context, param model.CreateUserParam) (*model.User, error)
}

var _ CreateUserUseCase = &CreateUserInteractor{}

type CreateUserInteractor struct {
	userRepo repository.UserRepository
	profileRepo repository.ProfileRepository
}

func NewCreateUserUseCase(userRepo repository.UserRepository , profileRepo repository.ProfileRepository) CreateUserUseCase {
	return &CreateUserInteractor{
		userRepo: userRepo,
		profileRepo: profileRepo,
	}
}

func (uc *CreateUserInteractor) Execute(ctx context.Context, param model.CreateUserParam) (*model.User, error) {
	user := &model.User{}
	err := user.CreateUser(param)
	if err != nil {
		return nil, err
	}

	// Set default values if not provided
	if user.Role == "" {
		user.Role = "user"
	}
	if user.Status == "" {
		user.Status = "active"
	}

	err = uc.userRepo.SaveUser(ctx, user)
	if err != nil {
		return nil, err
	}

	// 登録されたばかりのユーザーのIDを使って、空のプロフィールを作る
	emptyProfile := &model.Profile{
		UserID:    user.ID, // 先ほど作られたユーザーの内部ID
		Bio:       "",
		Grade:     0,       // 未設定は0とする
		Image:     "",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// プロフィール倉庫係にお願いして保存する
	if err := uc.profileRepo.SaveProfile(ctx, emptyProfile); err != nil {
		return nil, err
	}

	return user, nil


}
