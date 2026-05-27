package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SearchUsersUseCase interface {
	Execute(ctx context.Context, keyword string) ([]*model.User, error)
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

func (uc *SearchUsersInteractor) Execute(ctx context.Context, keyword string) ([]*model.User, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, err
	}

	users, err := uc.userRepo.SearchUsersByKeyword(ctx, keyword)
	if err != nil {
		return nil, err
	}

	return users, nil
}
