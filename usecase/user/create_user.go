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
	userRepo    repository.UserRepository
	profileRepo repository.ProfileRepository
	txManager   repository.TxManager
}

func NewCreateUserUseCase(userRepo repository.UserRepository, profileRepo repository.ProfileRepository, txManager repository.TxManager) CreateUserUseCase {
	return &CreateUserInteractor{
		userRepo:    userRepo,
		profileRepo: profileRepo,
		txManager:   txManager,
	}
}

func (uc *CreateUserInteractor) Execute(ctx context.Context, param model.CreateUserParam) (*model.User, error) {
	now := time.Now()
	param.CreatedAt = now
	param.UpdatedAt = now

	user := &model.User{}
	if err := user.CreateUser(param); err != nil {
		return nil, err
	}
	if user.Role == "" {
		user.Role = "user"
	}
	if user.Status == "" {
		user.Status = "active"
	}

	err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		if err := uc.userRepo.SaveUser(ctx, user); err != nil {
			return err
		}
		emptyProfile := &model.Profile{
			UserID:    user.ID,
			Bio:       "",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := uc.profileRepo.SaveProfile(ctx, emptyProfile); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return user, nil
}
