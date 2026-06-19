package user

import (
	"context"
	"errors"

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
	txManager     repository.TxManager
}

func NewDeleteUserUseCase(userRepo repository.UserRepository, postRepo repository.PostRepository, communityRepo repository.CommunityRepository, txManager repository.TxManager) DeleteUserUseCase {
	return &DeleteUserInteractor{
		userRepo:      userRepo,
		postRepo:      postRepo,
		communityRepo: communityRepo,
		txManager:     txManager,
	}
}

func (uc *DeleteUserInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	var deleted bool
	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		isSole, err := uc.communityRepo.IsSoleOwnerWithOtherMembers(ctx, id)
		if err != nil {
			return err
		}
		if isSole {
			return errors.New("cannot delete user: they are the sole owner of a community with other members; transfer ownership first")
		}

		if err := uc.postRepo.DeletePostsByUserID(ctx, id); err != nil {
			return err
		}
		if _, err := uc.communityRepo.DeleteCommunitiesWhereOnlyMember(ctx, id); err != nil {
			return err
		}
		deleted, err = uc.userRepo.DeleteUser(ctx, id)
		if err != nil {
			return err
		}
		if !deleted {
			return nil
		}
		return uc.postRepo.RecalculateReplyCountsAffectedByUser(ctx, id)
	}); err != nil {
		return false, err
	}
	return deleted, nil
}
