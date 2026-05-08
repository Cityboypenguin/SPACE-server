package user

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteUserUseCase interface {
	Execute(ctx context.Context, id int64) (bool, error)
}

var _ DeleteUserUseCase = &DeleteUserInteractor{}

type DeleteUserInteractor struct {
	userRepo repository.UserRepository
	postRepo repository.PostRepository
}

func NewDeleteUserUseCase(userRepo repository.UserRepository, postRepo repository.PostRepository) DeleteUserUseCase {
	return &DeleteUserInteractor{
		userRepo: userRepo,
		postRepo: postRepo,
	}
}

func (uc *DeleteUserInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	if err := uc.postRepo.DeletePostsByUserID(ctx, id); err != nil {
		return false, err
	}
	return uc.userRepo.DeleteUser(ctx, id)
}
