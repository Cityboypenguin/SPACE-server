package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListUsersUseCase interface {
	Execute(ctx context.Context, limit, offset int) ([]*model.User, int, error)
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

func (uc *ListUsersInteractor) Execute(ctx context.Context, limit, offset int) ([]*model.User, int, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}

	return uc.userRepo.ListUsers(ctx, limit, offset)
}
