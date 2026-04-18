package profile

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdateProfileUseCase interface {
	Execute(ctx context.Context, userID int64, param model.UpdateProfileParam) (*model.Profile, error)
}

type UpdateProfileInteractor struct {
	profileRepo repository.ProfileRepository
}

func NewUpdateProfileUseCase(profileRepo repository.ProfileRepository) UpdateProfileUseCase {
	return &UpdateProfileInteractor{
		profileRepo: profileRepo,
	}
}

func (uc *UpdateProfileInteractor) Execute(ctx context.Context, userID int64, param model.UpdateProfileParam) (*model.Profile, error) {
	// 1. 倉庫係に「この人のプロフィール、もうデータベースにある？」と確認する
	profile, err := uc.profileRepo.GetProfileByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 2. まだ無ければ、新しく空っぽのプロフィールを作成する
	if profile == nil {
		profile = &model.Profile{
			UserID:    userID,
			CreatedAt: time.Now().Unix(),
		}
	}

	// 3. 受け取ったデータ(param)で、プロフィールの中身を上書きする
	profile.UpdateProfile(param)

	// 4. 倉庫係に「これをデータベースに保存して！」とお願いする
	err = uc.profileRepo.SaveProfile(ctx, profile)
	if err != nil {
		return nil, err
	}

	// 5. 完成したプロフィールを受付係(Resolver)に返す
	return profile, nil
}