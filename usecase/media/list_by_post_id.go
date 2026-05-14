package media

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListMediaByPostIDUseCase interface {
	Execute(ctx context.Context, postID int64) ([]*model.Media, error)
}

var _ ListMediaByPostIDUseCase = &ListMediaByPostIDInteractor{}

type ListMediaByPostIDInteractor struct {
	mediaRepo repository.MediaRepository
}

func NewListMediaByPostIDUseCase(mediaRepo repository.MediaRepository) ListMediaByPostIDUseCase {
	return &ListMediaByPostIDInteractor{mediaRepo: mediaRepo}
}

func (uc *ListMediaByPostIDInteractor) Execute(ctx context.Context, postID int64) ([]*model.Media, error) {
	return uc.mediaRepo.ListByPostID(ctx, postID)
}
