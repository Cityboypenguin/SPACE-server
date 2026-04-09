package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListUsersUseCase interface {
	Execute(ctx context.Context) ([]*model.User, error)
}

var _ ListUsersUseCase = &ListUsersInteractor{}

type ListUsersInteractor struct {
	userRepo repository.UserRepository
}

func NewListUsersUseCase(userRepo repository.UserRepository) ListUsersUseCase {
	return &ListUsersInteractor{
		userRepo: userRepo,
	}
}

func (uc *ListUsersInteractor) Execute(ctx context.Context) ([]*model.User, error) {
	return uc.userRepo.ListUsers(ctx)
}
