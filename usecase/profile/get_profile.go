package profile

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetProfileUseCase interface {
	Execute(ctx context.Context, userID string) (*model.Profile, error)
}

type GetProfileInteractor struct {
	profileRepo repository.ProfileRepository
}

func NewGetProfileUseCase(profileRepo repository.ProfileRepository) GetProfileUseCase {
	return &GetProfileInteractor{
		profileRepo: profileRepo,
	}
}

func (uc *GetProfileInteractor) Execute(ctx context.Context, userID string) (*model.Profile, error) {
	// 取得はシンプルに、倉庫係から取り出したデータをそのまま返すだけです
	return uc.profileRepo.GetProfileByUserID(ctx, userID)
}