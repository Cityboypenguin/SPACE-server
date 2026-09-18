package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ListAnswerPagesByQuestionIDsUseCase は質問IDの集合について、同じ limit/offset の
// 回答ページと総件数をまとめて引く（DataLoader 用）。
//
// ListAnswersUseCase を質問ごとに呼ぶと、1質問あたり2クエリ（一覧 + COUNT）が
// 質問の数だけ走る。ここは質問が何件でも2クエリで済む。
//
// limit/offset を引数に残しているのは、GraphQL の answers フィールドが
// クライアントから件数を受け取るため。DataLoader のキーは
// (questionID, limit, offset) の組になる（dataloader.AnswerPageKey）。
type ListAnswerPagesByQuestionIDsUseCase interface {
	Execute(ctx context.Context, questionIDs []int64, q repository.PageQuery) (map[int64]*repository.AnswerPage, error)
}

var _ ListAnswerPagesByQuestionIDsUseCase = &ListAnswerPagesByQuestionIDsInteractor{}

type ListAnswerPagesByQuestionIDsInteractor struct {
	answerRepo repository.AnswerRepository
}

func NewListAnswerPagesByQuestionIDsUseCase(answerRepo repository.AnswerRepository) ListAnswerPagesByQuestionIDsUseCase {
	return &ListAnswerPagesByQuestionIDsInteractor{answerRepo: answerRepo}
}

// Execute mirrors ListAnswersUseCase.Execute: 認証必須、並びも同じ。
// 回答が無い質問も「空・Total 0」のページとして返る。
func (uc *ListAnswerPagesByQuestionIDsInteractor) Execute(ctx context.Context, questionIDs []int64, q repository.PageQuery) (map[int64]*repository.AnswerPage, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	return uc.answerRepo.ListAnswerPagesByQuestionIDs(ctx, questionIDs, claims.ID, q)
}
