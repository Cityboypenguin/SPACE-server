package user

import (
	"context"
	"os"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreateUserUseCase interface {
	Execute(ctx context.Context, param model.CreateUserParam) (*model.User, error)
}

var _ CreateUserUseCase = &CreateUserInteractor{}

type CreateUserInteractor struct {
	userRepo          repository.UserRepository
	profileRepo       repository.ProfileRepository
	txManager         repository.TxManager
	validationEnabled bool
}

func NewCreateUserUseCase(userRepo repository.UserRepository, profileRepo repository.ProfileRepository, txManager repository.TxManager) CreateUserUseCase {
	return &CreateUserInteractor{
		userRepo:          userRepo,
		profileRepo:       profileRepo,
		txManager:         txManager,
		validationEnabled: os.Getenv("DISABLE_USER_VALIDATION") != "true",
	}
}

func (uc *CreateUserInteractor) Execute(ctx context.Context, param model.CreateUserParam) (*model.User, error) {
	if uc.validationEnabled {
		if err := model.ValidateUserEmail(param.Email); err != nil {
			return nil, err
		}
		if err := model.ValidateUserPassword(param.Password); err != nil {
			return nil, err
		}
	}

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
		return uc.profileRepo.SaveProfile(ctx, emptyProfile)
	})
	if err != nil {
		return nil, err
	}

	return user, nil
}
