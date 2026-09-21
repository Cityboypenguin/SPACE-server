package question

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteQuestionUseCase interface {
	Execute(ctx context.Context, questionID int64) (bool, error)
}

var _ DeleteQuestionUseCase = &DeleteQuestionInteractor{}

type DeleteQuestionInteractor struct {
	events       EventPublisher
	questionRepo repository.QuestionRepository
}

func NewDeleteQuestionUseCase(events EventPublisher, questionRepo repository.QuestionRepository) DeleteQuestionUseCase {
	return &DeleteQuestionInteractor{events: orNoop(events), questionRepo: questionRepo}
}

// Execute は管理者のモデレーションとして質問を消す（回答も ON DELETE CASCADE で消える）。
// 本人による削除は DeleteMyQuestionUseCase。
//
// 管理者かどうかの判定をここに置くのは、他の削除経路と揃えるため。呼び出し側の
// 手順にしておくと、このユースケースを直接呼ぶ入口（別のAPI・バッチ・管理用の
// コマンド）を足したときに、そこだけ判定が抜ける。
func (uc *DeleteQuestionInteractor) Execute(ctx context.Context, questionID int64) (bool, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return false, err
	}

	// 配信にはどの部屋の質問かが要るので、消す前に引いておく。消した後では引けない。
	q, err := uc.questionRepo.GetQuestionByID(ctx, questionID)
	if err != nil {
		return false, err
	}
	if q == nil {
		// 既に無いものを消そうとしただけ。DeleteQuestion 自身も同じく false を返す。
		return false, nil
	}

	ok, err := uc.questionRepo.DeleteQuestion(ctx, questionID)
	if err != nil || !ok {
		return ok, err
	}

	// 配信はここで出す。リゾルバの手順にしておくと、リゾルバを通らない経路では
	// 購読中の画面が動かない（usecase/*/events.go 参照）。管理者が消した質問が
	// 開いたままの画面に残り続けるのは、まさにこの形で起きていた。
	uc.events.QuestionDeleted(ctx, q)
	return true, nil
}
