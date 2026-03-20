package user

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SignUpUseCase interface {
	Execute(ctx context.Context, name string) (*model.User, error)
}

var _ SignUpUseCase = &SignUpInteractor{}

type SignUpInteractor struct {
	UserRepository repository.UserRepository
}

func (i *SignUpInteractor) Execute(ctx context.Context, name string) (*model.User, error) {
	now := time.Now()
	u := model.NewUser(model.CreateUserParam{
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	})

	if err := i.UserRepository.SaveUser(ctx, u); err != nil {
		return nil, err
	}

	return u, nil
}
