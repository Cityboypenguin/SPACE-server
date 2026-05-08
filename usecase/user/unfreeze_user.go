package user

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UnfreezeUserUseCase interface {
	Execute(ctx context.Context, id int64) (bool, error)
}

var _ UnfreezeUserUseCase = &UnfreezeUserInteractor{}

type UnfreezeUserInteractor struct {
	userRepo repository.UserRepository
}

func NewUnfreezeUserUseCase(userRepo repository.UserRepository) UnfreezeUserUseCase {
	return &UnfreezeUserInteractor{userRepo: userRepo}
}

func (uc *UnfreezeUserInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	u, err := uc.userRepo.GetUserByID(ctx, id)
	if err != nil {
		return false, err
	}
	if u == nil {
		return false, errors.New("user not found")
	}
	if u.Status != model.UserStatusFrozen {
		return false, errors.New("user is not frozen")
	}

	u.Status = model.UserStatusActive
	if err := uc.userRepo.UpdateUser(ctx, u); err != nil {
		return false, err
	}
	return true, nil
}
