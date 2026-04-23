package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetUserByIDUseCase interface {
	Execute(ctx context.Context, id int64) (*model.User, error)
}

var _ GetUserByIDUseCase = &GetUserByIDInteractor{}

type GetUserByIDInteractor struct {
	userRepo repository.UserRepository
}

func NewGetUserByIDUseCase(userRepo repository.UserRepository) GetUserByIDUseCase {
	return &GetUserByIDInteractor{
		userRepo: userRepo,
	}
}

func (uc *GetUserByIDInteractor) Execute(ctx context.Context, id int64) (*model.User, error) {
	if _, err := authz.RequireSelfOrAdmin(ctx, id); err != nil {
		return nil, err
	}

	return uc.userRepo.GetUserByID(ctx, id)
}
