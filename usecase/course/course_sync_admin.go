package course

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// CourseSyncAdminUseCase は管理画面からシラバス同期の結果（実行・変更）と
// 確認待ちを見て、確認に判断を付けるためのもの。
type CourseSyncAdminUseCase interface {
	ListRuns(ctx context.Context, year *int, page repository.PageQuery) ([]*model.CourseSyncRun, int, error)
	GetRun(ctx context.Context, id int64) (*model.CourseSyncRun, error)
	ListChanges(ctx context.Context, runID int64, kind *model.CourseSyncChangeKind, page repository.PageQuery) ([]*model.CourseSyncChange, int, error)
	ListReviews(ctx context.Context, param repository.ListCourseSyncReviewsParam) ([]*model.CourseSyncReview, int, error)
	// ResolveReview は確認待ちの確認に判断（SAME / DIFFERENT / IGNORED）を記録する。
	// 判断は次の同期で反映される（ここでは授業に触らない。同期と同じ照合を
	// 通すことで、反映のされ方を同期のサマリーで確かめられるようにするため）。
	ResolveReview(ctx context.Context, id int64, decision model.CourseSyncReviewStatus) (*model.CourseSyncReview, error)
}

var _ CourseSyncAdminUseCase = &CourseSyncAdminInteractor{}

type CourseSyncAdminInteractor struct {
	syncRepo repository.CourseSyncRepository
	now      func() time.Time
}

func NewCourseSyncAdminUseCase(syncRepo repository.CourseSyncRepository) CourseSyncAdminUseCase {
	return &CourseSyncAdminInteractor{syncRepo: syncRepo, now: time.Now}
}

func (uc *CourseSyncAdminInteractor) ListRuns(ctx context.Context, year *int, page repository.PageQuery) ([]*model.CourseSyncRun, int, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}
	return uc.syncRepo.ListRuns(ctx, year, page)
}

func (uc *CourseSyncAdminInteractor) GetRun(ctx context.Context, id int64) (*model.CourseSyncRun, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	return uc.syncRepo.GetRun(ctx, id)
}

func (uc *CourseSyncAdminInteractor) ListChanges(ctx context.Context, runID int64, kind *model.CourseSyncChangeKind, page repository.PageQuery) ([]*model.CourseSyncChange, int, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}
	return uc.syncRepo.ListChanges(ctx, runID, kind, page)
}

func (uc *CourseSyncAdminInteractor) ListReviews(ctx context.Context, param repository.ListCourseSyncReviewsParam) ([]*model.CourseSyncReview, int, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}
	return uc.syncRepo.ListReviews(ctx, param)
}

func (uc *CourseSyncAdminInteractor) ResolveReview(ctx context.Context, id int64, decision model.CourseSyncReviewStatus) (*model.CourseSyncReview, error) {
	claims, err := authz.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	switch decision {
	case model.CourseSyncReviewSame, model.CourseSyncReviewDifferent, model.CourseSyncReviewIgnored:
	default:
		return nil, apperr.InvalidInput("判断が不正です")
	}

	review, err := uc.syncRepo.GetReview(ctx, id)
	if err != nil {
		return nil, err
	}
	if review == nil {
		return nil, apperr.NotFound("確認が見つかりません")
	}
	// コマの対応が決まらないものは「同じ授業」と言われても、どのコマをどのコマへ
	// 移すかが決まらない。作り直すか据え置くかのどちらかにしてもらう。
	if review.Kind == model.CourseSyncReviewSlotAmbiguous && decision == model.CourseSyncReviewSame {
		return nil, apperr.InvalidInput("コマの対応が決まらない確認は「作り直す」か「据え置く」のどちらかを選んでください")
	}

	ok, err := uc.syncRepo.ResolveReview(ctx, id, decision, claims.ID, uc.now())
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.Conflict("この確認は既に判断済みです")
	}
	return uc.syncRepo.GetReview(ctx, id)
}
