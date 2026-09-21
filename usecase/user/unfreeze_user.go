package user

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
)

type UnfreezeUserUseCase interface {
	Execute(ctx context.Context, id int64) (bool, error)
}

var _ UnfreezeUserUseCase = &UnfreezeUserInteractor{}

type UnfreezeUserInteractor struct {
	userRepo userStatusRepository
}

func NewUnfreezeUserUseCase(userRepo userStatusRepository) UnfreezeUserUseCase {
	return &UnfreezeUserInteractor{userRepo: userRepo}
}

func (uc *UnfreezeUserInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return false, err
	}
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
