package media

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListMediaByMessageIDsUseCase interface {
	Execute(ctx context.Context, messageIDs []int64) (map[int64][]*model.Media, error)
}

var _ ListMediaByMessageIDsUseCase = &ListMediaByMessageIDsInteractor{}

type ListMediaByMessageIDsInteractor struct {
	mediaRepo repository.MediaRepository
}

func NewListMediaByMessageIDsUseCase(mediaRepo repository.MediaRepository) ListMediaByMessageIDsUseCase {
	return &ListMediaByMessageIDsInteractor{mediaRepo: mediaRepo}
}

func (uc *ListMediaByMessageIDsInteractor) Execute(ctx context.Context, messageIDs []int64) (map[int64][]*model.Media, error) {
	return uc.mediaRepo.ListByMessageIDs(ctx, messageIDs)
}
