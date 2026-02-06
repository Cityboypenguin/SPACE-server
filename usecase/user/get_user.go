package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetUserUseCase interface {
	Execute(ctx context.Context, id int64) (*model.User, error)
}

var _ GetUserUseCase = &GetUserInteractor{}

type GetUserInteractor struct {
	UserRepository repository.UserRepository
}

func (i *GetUserInteractor) Execute(ctx context.Context, id int64) (*model.User, error) {
	return i.UserRepository.GetUser(ctx, id)
}
