package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SearchUsersUseCase interface {
	Execute(ctx context.Context, name string) ([]*model.User, error)
}

var _ SearchUsersUseCase = &SearchUsersInteractor{}

type SearchUsersInteractor struct {
	userRepo repository.UserRepository
}

func NewSearchUsersUseCase(userRepo repository.UserRepository) SearchUsersUseCase {
	return &SearchUsersInteractor{
		userRepo: userRepo,
	}
}

func (uc *SearchUsersInteractor) Execute(ctx context.Context, name string) ([]*model.User, error) {
	users, err := uc.userRepo.SearchUsersByName(ctx, name)
	if err != nil {
		return nil, err
	}

	return users, nil
}
