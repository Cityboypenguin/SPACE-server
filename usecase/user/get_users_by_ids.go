package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetUsersByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) ([]*model.User, error)
}

var _ GetUsersByIDsUseCase = &GetUsersByIDsInteractor{}

type GetUsersByIDsInteractor struct {
	userRepo repository.UserRepository
}

func NewGetUsersByIDsUseCase(userRepo repository.UserRepository) GetUsersByIDsUseCase {
	return &GetUsersByIDsInteractor{userRepo: userRepo}
}

func (uc *GetUsersByIDsInteractor) Execute(ctx context.Context, ids []int64) ([]*model.User, error) {
	return uc.userRepo.GetUsersByIDs(ctx, ids)
}
