package poll

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ListPollsQuery は Polls 一覧が「要求された派生値だけ」を計算するためのオプション。
//
// PollPage だけは total のほかに unvotedTotal（自分がまだ投票していない数）を持つ。
// どちらも GraphQL が選んだときだけ数えたいが、Execute の引数に bool を並べると
// 呼び出しが Execute(ctx, rid, 50, 0, true, false) のように読めなくなるので、
// ページング共通の repository.PageQuery に、この一覧固有の1つを足した形にした
// （引数の増やし方を型に固定するという点で PageQuery と同じ方針）。
type ListPollsQuery struct {
	// Page は窓と「total を数えるか」。
	Page repository.PageQuery
	// WithUnvotedTotal が true のときだけ未投票数を数える。false なら 0 を返す。
	WithUnvotedTotal bool
}

type ListPollsUseCase interface {
	Execute(ctx context.Context, roomID int64, q ListPollsQuery) ([]*model.Poll, int, int, error)
}

var _ ListPollsUseCase = &ListPollsInteractor{}

type ListPollsInteractor struct {
	pollRepo repository.PollRepository
}

func NewListPollsUseCase(pollRepo repository.PollRepository) ListPollsUseCase {
	return &ListPollsInteractor{pollRepo: pollRepo}
}

func (uc *ListPollsInteractor) Execute(ctx context.Context, roomID int64, q ListPollsQuery) ([]*model.Poll, int, int, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	items, total, err := uc.pollRepo.ListPollsByRoomID(ctx, roomID, q.Page)
	if err != nil {
		return nil, 0, 0, err
	}
	// 未投票数は poll_options と poll_votes を跨ぐ NOT EXISTS なので、一覧本体の
	// SELECT より重い。unvotedTotal を選んでいないクライアント（投票一覧を
	// 表示するだけの画面）には撃たない。
	var unvotedTotal int
	if q.WithUnvotedTotal {
		unvotedTotal, err = uc.pollRepo.CountUnvotedPollsByRoomID(ctx, roomID, claims.ID)
		if err != nil {
			return nil, 0, 0, err
		}
	}
	return items, total, unvotedTotal, nil
}
