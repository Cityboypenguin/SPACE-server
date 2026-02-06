package user

import (
	"context"
	"time"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SignUpUseCase interface {
	Execute(context.Context, gqlmodel.SignUpInput) (*model.User, error)
}

var _ SignUpUseCase = &SignUpInteractor{}

type SignUpInteractor struct {
	UserRepository repository.UserRepository
}

func (i *SignUpInteractor) Execute(ctx context.Context, in gqlmodel.SignUpInput) (*model.User, error) {
	now := time.Now()
	u := &model.User{}
	err := u.Create(model.CreateUserParam{
		Name:      in.Name,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return nil, err
	}

	if err := i.UserRepository.SaveUser(ctx, u); err != nil {
		return nil, err
	}

	return u, nil
}
