package media

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListMediaByQuestionIDsUseCase interface {
	Execute(ctx context.Context, questionIDs []int64) (map[int64][]*model.Media, error)
}

var _ ListMediaByQuestionIDsUseCase = &ListMediaByQuestionIDsInteractor{}

type ListMediaByQuestionIDsInteractor struct {
	mediaRepo repository.MediaRepository
}

func NewListMediaByQuestionIDsUseCase(mediaRepo repository.MediaRepository) ListMediaByQuestionIDsUseCase {
	return &ListMediaByQuestionIDsInteractor{mediaRepo: mediaRepo}
}

func (uc *ListMediaByQuestionIDsInteractor) Execute(ctx context.Context, questionIDs []int64) (map[int64][]*model.Media, error) {
	return uc.mediaRepo.ListByQuestionIDs(ctx, questionIDs)
}
