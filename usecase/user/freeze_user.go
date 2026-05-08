package user

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type FreezeUserUseCase interface {
	Execute(ctx context.Context, id int64) (bool, error)
}

var _ FreezeUserUseCase = &FreezeUserInteractor{}

type FreezeUserInteractor struct {
	userRepo repository.UserRepository
}

func NewFreezeUserUseCase(userRepo repository.UserRepository) FreezeUserUseCase {
	return &FreezeUserInteractor{userRepo: userRepo}
}

func (uc *FreezeUserInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	u, err := uc.userRepo.GetUserByID(ctx, id)
	if err != nil {
		return false, err
	}
	if u == nil {
		return false, errors.New("user not found")
	}
	if u.Status == model.UserStatusFrozen {
		return false, errors.New("user is already frozen")
	}

	u.Status = model.UserStatusFrozen
	if err := uc.userRepo.UpdateUser(ctx, u); err != nil {
		return false, err
	}
	return true, nil
}
