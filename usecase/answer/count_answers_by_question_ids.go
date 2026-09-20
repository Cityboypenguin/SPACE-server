package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// CountAnswersByQuestionIDsUseCase は質問ごとの回答件数を数える。
//
// 回答一覧（ListAnswerPagesByQuestionIDs）と分けてあるのは、件数しか要らない
// 画面のため。一覧で代用すると、件数を出すためだけに回答本文と投稿者が
// 回答の数だけ応答に乗る。
type CountAnswersByQuestionIDsUseCase interface {
	Execute(ctx context.Context, questionIDs []int64) (map[int64]int, error)
}

var _ CountAnswersByQuestionIDsUseCase = &CountAnswersByQuestionIDsInteractor{}

type CountAnswersByQuestionIDsInteractor struct {
	answerRepo repository.AnswerRepository
}

func NewCountAnswersByQuestionIDsUseCase(answerRepo repository.AnswerRepository) CountAnswersByQuestionIDsUseCase {
	return &CountAnswersByQuestionIDsInteractor{answerRepo: answerRepo}
}

func (uc *CountAnswersByQuestionIDsInteractor) Execute(ctx context.Context, questionIDs []int64) (map[int64]int, error) {
	return uc.answerRepo.CountAnswersByQuestionIDs(ctx, questionIDs)
}
