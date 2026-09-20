package question

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// GetQuestionsByIDsUseCase は質問IDの集合から質問をまとめて引く（DataLoader 用）。
// 単体の GetQuestionByIDUseCase と同じく認証必須。
type GetQuestionsByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) (map[int64]*model.Question, error)
}

var _ GetQuestionsByIDsUseCase = &GetQuestionsByIDsInteractor{}

type GetQuestionsByIDsInteractor struct {
	questionRepo repository.QuestionRepository
}

func NewGetQuestionsByIDsUseCase(questionRepo repository.QuestionRepository) GetQuestionsByIDsUseCase {
	return &GetQuestionsByIDsInteractor{questionRepo: questionRepo}
}

// Execute mirrors GetQuestionByIDUseCase.Execute: 未認証は同じ理由で弾く。
// 見つからない ID は map に入らない（エラーにはしない）。
func (uc *GetQuestionsByIDsInteractor) Execute(ctx context.Context, ids []int64) (map[int64]*model.Question, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, err
	}
	return uc.questionRepo.GetQuestionsByIDs(ctx, ids)
}
