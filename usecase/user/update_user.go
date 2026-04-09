package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdateUserUseCase interface {
	Execute(ctx context.Context, id int64, param model.UpdateUserParam) (*model.User, error)
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

func (uc *UpdateUserInteractor) Execute(ctx context.Context, id int64, param model.UpdateUserParam) (*model.User, error) {
	user, err := uc.userRepo.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}

	err = user.UpdateUser(param)
	if err != nil {
		return nil, err
	}

	err = uc.userRepo.SaveUser(ctx, user)
	if err != nil {
		return nil, err
	}

	return user, nil
}
