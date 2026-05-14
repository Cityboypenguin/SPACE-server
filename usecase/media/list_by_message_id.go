package media

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListMediaByMessageIDUseCase interface {
	Execute(ctx context.Context, messageID int64) ([]*model.Media, error)
}

var _ ListMediaByMessageIDUseCase = &ListMediaByMessageIDInteractor{}

type ListMediaByMessageIDInteractor struct {
	mediaRepo repository.MediaRepository
}

func NewListMediaByMessageIDUseCase(mediaRepo repository.MediaRepository) ListMediaByMessageIDUseCase {
	return &ListMediaByMessageIDInteractor{mediaRepo: mediaRepo}
}

func (uc *ListMediaByMessageIDInteractor) Execute(ctx context.Context, messageID int64) ([]*model.Media, error) {
	return uc.mediaRepo.ListByMessageID(ctx, messageID)
}
