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
	UserRepository repository.UserRepository
}

func (i *GetUsersByIDsInteractor) Execute(ctx context.Context, ids []int64) ([]*model.User, error) {
	return i.UserRepository.GetUsersByIDs(ctx, ids)
}
