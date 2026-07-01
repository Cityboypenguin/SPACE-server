package user

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type AdminCreateUserUseCase interface {
	Execute(ctx context.Context, param model.CreateUserParam) (*model.User, error)
}

var _ AdminCreateUserUseCase = &AdminCreateUserInteractor{}

type AdminCreateUserInteractor struct {
	userRepo    repository.UserRepository
	profileRepo repository.ProfileRepository
	txManager   repository.TxManager
}

func NewAdminCreateUserUseCase(userRepo repository.UserRepository, profileRepo repository.ProfileRepository, txManager repository.TxManager) AdminCreateUserUseCase {
	return &AdminCreateUserInteractor{
		userRepo:    userRepo,
		profileRepo: profileRepo,
		txManager:   txManager,
	}
}

func (uc *AdminCreateUserInteractor) Execute(ctx context.Context, param model.CreateUserParam) (*model.User, error) {
	if err := model.ValidateUserPassword(param.Password); err != nil {
		return nil, err
	}

	now := time.Now()
	param.CreatedAt = now
	param.UpdatedAt = now

	user := &model.User{}
	if err := user.CreateUser(param); err != nil {
		return nil, err
	}
	user.Role = "user"
	user.Status = "active"

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
		return uc.profileRepo.SaveProfile(ctx, emptyProfile)
	})
	if err != nil {
		return nil, err
	}

	return user, nil
}
