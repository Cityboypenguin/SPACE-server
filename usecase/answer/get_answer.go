package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetAnswerByIDUseCase interface {
	Execute(ctx context.Context, id int64) (*repository.AnswerWithLikes, error)
}

var _ GetAnswerByIDUseCase = &GetAnswerByIDInteractor{}

type GetAnswerByIDInteractor struct {
	answerRepo repository.AnswerRepository
}

func NewGetAnswerByIDUseCase(answerRepo repository.AnswerRepository) GetAnswerByIDUseCase {
	return &GetAnswerByIDInteractor{answerRepo: answerRepo}
}

func (uc *GetAnswerByIDInteractor) Execute(ctx context.Context, id int64) (*repository.AnswerWithLikes, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	return uc.answerRepo.GetAnswerWithLikesByID(ctx, id, claims.ID)
}
