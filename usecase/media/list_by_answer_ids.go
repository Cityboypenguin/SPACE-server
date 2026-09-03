package media

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListMediaByAnswerIDsUseCase interface {
	Execute(ctx context.Context, answerIDs []int64) (map[int64][]*model.Media, error)
}

var _ ListMediaByAnswerIDsUseCase = &ListMediaByAnswerIDsInteractor{}

type ListMediaByAnswerIDsInteractor struct {
	mediaRepo repository.MediaRepository
}

func NewListMediaByAnswerIDsUseCase(mediaRepo repository.MediaRepository) ListMediaByAnswerIDsUseCase {
	return &ListMediaByAnswerIDsInteractor{mediaRepo: mediaRepo}
}

func (uc *ListMediaByAnswerIDsInteractor) Execute(ctx context.Context, answerIDs []int64) (map[int64][]*model.Media, error) {
	return uc.mediaRepo.ListByAnswerIDs(ctx, answerIDs)
}
