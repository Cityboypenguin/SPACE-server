package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteUserUseCase interface {
	Execute(ctx context.Context, id int64) (bool, error)
}

var _ DeleteUserUseCase = &DeleteUserInteractor{}

type DeleteUserInteractor struct {
	userRepo repository.UserRepository
}

func NewDeleteUserUseCase(userRepo repository.UserRepository) DeleteUserUseCase {
	return &DeleteUserInteractor{
		userRepo: userRepo,
	}
}

func (uc *DeleteUserInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	return uc.userRepo.DeleteUser(ctx, id)
}
