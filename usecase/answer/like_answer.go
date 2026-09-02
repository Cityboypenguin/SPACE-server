package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type LikeAnswerUseCase interface {
	Execute(ctx context.Context, answerID int64) (*repository.AnswerWithLikes, error)
}

var _ LikeAnswerUseCase = &LikeAnswerInteractor{}

type LikeAnswerInteractor struct {
	answerRepo repository.AnswerRepository
}

func NewLikeAnswerUseCase(answerRepo repository.AnswerRepository) LikeAnswerUseCase {
	return &LikeAnswerInteractor{answerRepo: answerRepo}
}

func (uc *LikeAnswerInteractor) Execute(ctx context.Context, answerID int64) (*repository.AnswerWithLikes, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	a, err := uc.answerRepo.GetAnswerByID(ctx, answerID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, apperr.NotFound("回答が見つかりません")
	}

	if err := uc.answerRepo.LikeAnswer(ctx, answerID, claims.ID); err != nil {
		return nil, err
	}
	return uc.answerRepo.GetAnswerWithLikesByID(ctx, answerID, claims.ID)
}

type UnlikeAnswerUseCase interface {
	Execute(ctx context.Context, answerID int64) (*repository.AnswerWithLikes, error)
}

var _ UnlikeAnswerUseCase = &UnlikeAnswerInteractor{}

type UnlikeAnswerInteractor struct {
	answerRepo repository.AnswerRepository
}

func NewUnlikeAnswerUseCase(answerRepo repository.AnswerRepository) UnlikeAnswerUseCase {
	return &UnlikeAnswerInteractor{answerRepo: answerRepo}
}

func (uc *UnlikeAnswerInteractor) Execute(ctx context.Context, answerID int64) (*repository.AnswerWithLikes, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	a, err := uc.answerRepo.GetAnswerByID(ctx, answerID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, apperr.NotFound("回答が見つかりません")
	}

	if err := uc.answerRepo.UnlikeAnswer(ctx, answerID, claims.ID); err != nil {
		return nil, err
	}
	return uc.answerRepo.GetAnswerWithLikesByID(ctx, answerID, claims.ID)
}
