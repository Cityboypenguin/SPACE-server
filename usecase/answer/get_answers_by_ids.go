package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// GetAnswersByIDsUseCase は回答IDの集合から回答をまとめて引く（DataLoader 用）。
// 単体の GetAnswerByIDUseCase と同じく認証必須で、いいね数・自分がいいねしたかは
// 呼び出し元のユーザーで解決する。
type GetAnswersByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) (map[int64]*repository.AnswerWithLikes, error)
}

var _ GetAnswersByIDsUseCase = &GetAnswersByIDsInteractor{}

type GetAnswersByIDsInteractor struct {
	answerRepo repository.AnswerRepository
}

func NewGetAnswersByIDsUseCase(answerRepo repository.AnswerRepository) GetAnswersByIDsUseCase {
	return &GetAnswersByIDsInteractor{answerRepo: answerRepo}
}

// Execute mirrors GetAnswerByIDUseCase.Execute. 見つからない ID は map に入らない。
func (uc *GetAnswersByIDsInteractor) Execute(ctx context.Context, ids []int64) (map[int64]*repository.AnswerWithLikes, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	return uc.answerRepo.GetAnswersWithLikesByIDs(ctx, ids, claims.ID)
}
