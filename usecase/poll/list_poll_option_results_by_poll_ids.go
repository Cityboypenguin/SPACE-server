package poll

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ListPollOptionResultsByPollIDsUseCase は投票IDの集合の選択肢と集計をまとめて引く
// （DataLoader 用）。単体の ListPollOptionResultsUseCase と同じく認証必須で、
// 「自分が入れたか」は呼び出し元のユーザーで解決する。
type ListPollOptionResultsByPollIDsUseCase interface {
	Execute(ctx context.Context, pollIDs []int64) (map[int64][]*repository.PollOptionResult, error)
}

var _ ListPollOptionResultsByPollIDsUseCase = &ListPollOptionResultsByPollIDsInteractor{}

type ListPollOptionResultsByPollIDsInteractor struct {
	pollRepo repository.PollRepository
}

func NewListPollOptionResultsByPollIDsUseCase(pollRepo repository.PollRepository) ListPollOptionResultsByPollIDsUseCase {
	return &ListPollOptionResultsByPollIDsInteractor{pollRepo: pollRepo}
}

func (uc *ListPollOptionResultsByPollIDsInteractor) Execute(ctx context.Context, pollIDs []int64) (map[int64][]*repository.PollOptionResult, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	return uc.pollRepo.ListOptionsWithResultsByPollIDs(ctx, pollIDs, claims.ID)
}
