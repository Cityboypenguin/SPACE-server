package poll

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

type DeletePollUseCase interface {
	Execute(ctx context.Context, pollID int64) (*model.Poll, error)
}

var _ DeletePollUseCase = &DeletePollInteractor{}

type DeletePollInteractor struct {
	events          EventPublisher
	pollRepo        repository.PollRepository
	requireWritable course.RequireWritableCourseRoomUseCase
}

func NewDeletePollUseCase(events EventPublisher, pollRepo repository.PollRepository, requireWritable course.RequireWritableCourseRoomUseCase) DeletePollUseCase {
	return &DeletePollInteractor{events: orNoop(events), pollRepo: pollRepo, requireWritable: requireWritable}
}

// Execute deletes a poll (and its options/votes), allowed for the poll's own
// author or an administrator moderating 授業内チャット. It returns the poll as
// it was just before deletion, so the caller can notify subscribers.
//
// 作成・投票と同じく、書き込みできない授業（過去の学期、または時間割に未登録）では
// 削除もできない。管理者のモデレーションは学期に関係なく行えるべきなので、
// 管理者はこのチェックを通さない。
func (uc *DeletePollInteractor) Execute(ctx context.Context, pollID int64) (*model.Poll, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	p, err := uc.pollRepo.GetPollByID(ctx, pollID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, apperr.NotFound("投票が見つかりません")
	}
	isAdmin := authz.IsAdminRole(claims.Role)
	if p.AuthorUserID != claims.ID && !isAdmin {
		return nil, apperr.Forbidden("この投票を削除する権限がありません")
	}
	if !isAdmin {
		if _, err := uc.requireWritable.Execute(ctx, p.RoomID); err != nil {
			return nil, err
		}
	}

	if _, err := uc.pollRepo.DeletePoll(ctx, pollID); err != nil {
		return nil, err
	}
	// 配信はここで出す。リゾルバの手順にしておくと、リゾルバを通らない経路では
	// 購読中の画面が動かない（usecase/*/events.go 参照）。
	uc.events.PollDeleted(ctx, p)
	return p, nil
}
