package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreateUserUseCase interface {
	Execute(ctx context.Context, param model.CreateUserParam) (*model.User, error)
}

var _ CreateUserUseCase = &CreateUserInteractor{}

type CreateUserInteractor struct {
	userRepo repository.UserRepository
}

func NewCreateUserUseCase(userRepo repository.UserRepository) CreateUserUseCase {
	return &CreateUserInteractor{
		userRepo: userRepo,
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

	return user, nil
}
