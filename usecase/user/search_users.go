package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SearchUsersUseCase interface {
	Execute(ctx context.Context, keyword string, q repository.PageQuery) ([]*model.User, int, error)
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

func (uc *SearchUsersInteractor) Execute(ctx context.Context, keyword string, q repository.PageQuery) ([]*model.User, int, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, 0, err
	}

	users, total, err := uc.userRepo.SearchUsersByKeyword(ctx, keyword, q)
	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
}
