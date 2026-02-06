package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetUsersUseCase interface {
	Execute(context.Context) ([]*model.User, error)
}

var _ GetUsersUseCase = &GetUsersInteractor{}

type GetUsersInteractor struct {
	UserRepository repository.UserRepository
}

func (i *GetUsersInteractor) Execute(ctx context.Context) ([]*model.User, error) {
	return i.UserRepository.GetUsers(ctx)
}
