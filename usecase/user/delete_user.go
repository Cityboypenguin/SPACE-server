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
	userRepo      repository.UserRepository
	postRepo      repository.PostRepository
	communityRepo repository.CommunityRepository
}

func NewDeleteUserUseCase(userRepo repository.UserRepository, postRepo repository.PostRepository, communityRepo repository.CommunityRepository) DeleteUserUseCase {
	return &DeleteUserInteractor{
		userRepo:      userRepo,
		postRepo:      postRepo,
		communityRepo: communityRepo,
	}
}

func (uc *DeleteUserInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	if err := uc.postRepo.DeletePostsByUserID(ctx, id); err != nil {
		return false, err
	}
	if _, err := uc.communityRepo.DeleteCommunitiesWhereOnlyMember(ctx, id); err != nil {
		_ = uc.postRepo.RecalculateReplyCounts(ctx)
		return false, err
	}
	deleted, err := uc.userRepo.DeleteUser(ctx, id)
	if err != nil || !deleted {
		_ = uc.postRepo.RecalculateReplyCounts(ctx)
		return deleted, err
	}
	if err := uc.postRepo.RecalculateReplyCounts(ctx); err != nil {
		return false, err
	}
	return true, nil
}
