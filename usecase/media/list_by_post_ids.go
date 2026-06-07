package media

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListMediaByPostIDsUseCase interface {
	Execute(ctx context.Context, postIDs []int64) (map[int64][]*model.Media, error)
}

var _ ListMediaByPostIDsUseCase = &ListMediaByPostIDsInteractor{}

type ListMediaByPostIDsInteractor struct {
	mediaRepo repository.MediaRepository
}

func NewListMediaByPostIDsUseCase(mediaRepo repository.MediaRepository) ListMediaByPostIDsUseCase {
	return &ListMediaByPostIDsInteractor{mediaRepo: mediaRepo}
}

func (uc *ListMediaByPostIDsInteractor) Execute(ctx context.Context, postIDs []int64) (map[int64][]*model.Media, error) {
	return uc.mediaRepo.ListByPostIDs(ctx, postIDs)
}
