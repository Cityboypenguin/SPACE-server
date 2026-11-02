package graph

import (
	"context"
	"time"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// pollResultsHidden は、この呼び出し元に選択肢ごとの票数を伏せるかを決める。
//
// 回答期限のある投票は、期限が来るまで結果を伏せて、先に出た票に流されないようにする。
// 以前は画面だけで伏せていて、API を直接呼べば期限前でも票数が読めた。
// 伏せないのは、期限が無い・過ぎた・自分が投票済み・作成者・管理者のとき。
// 投票した人数（voterCount）は結果ではないので伏せない。
func pollResultsHidden(ctx context.Context, poll *gqlmodel.Poll, results []*repository.PollOptionResult, now time.Time) bool {
	if poll.Deadline == nil {
		return false
	}
	deadline, err := time.Parse(timeFormat, *poll.Deadline)
	if err != nil || !now.Before(deadline) {
		return false
	}
	if claims, ok := auth.ClaimsFromContext(ctx); ok && isAdminRole(claims.Role) {
		return false
	}
	if isCallerID(ctx, poll.User.ID) {
		return false
	}
	for _, res := range results {
		if res.VotedByMe {
			return false
		}
	}
	return true
}
