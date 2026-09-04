package profile

import (
	"context"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdateMyProfileParam struct {
	User    model.UpdateUserParam
	Profile model.UpdateProfileParam
}

type UpdateMyProfileUseCase interface {
	Execute(ctx context.Context, userID int64, param UpdateMyProfileParam) (*model.User, *model.Profile, error)
}

type UpdateMyProfileInteractor struct {
	userRepo    repository.UserRepository
	profileRepo repository.ProfileRepository
	txManager   repository.TxManager
}

func NewUpdateMyProfileUseCase(
	userRepo repository.UserRepository,
	profileRepo repository.ProfileRepository,
	txManager repository.TxManager,
) UpdateMyProfileUseCase {
	return &UpdateMyProfileInteractor{
		userRepo:    userRepo,
		profileRepo: profileRepo,
		txManager:   txManager,
	}
}

func (uc *UpdateMyProfileInteractor) Execute(ctx context.Context, userID int64, param UpdateMyProfileParam) (*model.User, *model.Profile, error) {
	var updatedUser *model.User
	var updatedProfile *model.Profile

	err := uc.txManager.RunInTx(ctx, func(txCtx context.Context) error {
		user, err := uc.userRepo.GetUserByID(txCtx, userID)
		if err != nil {
			return err
		}
		if user == nil {
			return fmt.Errorf("user not found")
		}

		if err := user.UpdateUser(param.User); err != nil {
			return err
		}
		if err := uc.userRepo.UpdateUser(txCtx, user); err != nil {
			return err
		}

		profile, err := uc.profileRepo.GetProfileByUserID(txCtx, userID)
		if err != nil {
			return err
		}
		if profile == nil {
			profile = &model.Profile{
				UserID:    userID,
				CreatedAt: time.Now(),
			}
		}
		profile.UpdateProfile(param.Profile)
		if err := uc.profileRepo.SaveProfile(txCtx, profile); err != nil {
			return err
		}

		updatedUser = user
		updatedProfile = profile
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return updatedUser, updatedProfile, nil
}
