package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetAnswerByIDUseCase interface {
	Execute(ctx context.Context, id int64) (*model.Answer, error)
}

var _ GetAnswerByIDUseCase = &GetAnswerByIDInteractor{}

type GetAnswerByIDInteractor struct {
	answerRepo repository.AnswerRepository
}

func NewGetAnswerByIDUseCase(answerRepo repository.AnswerRepository) GetAnswerByIDUseCase {
	return &GetAnswerByIDInteractor{answerRepo: answerRepo}
}

func (uc *GetAnswerByIDInteractor) Execute(ctx context.Context, id int64) (*model.Answer, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, err
	}
	return uc.answerRepo.GetAnswerByID(ctx, id)
}
