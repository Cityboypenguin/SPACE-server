package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type TermsRepository interface {
	Save(ctx context.Context, t *model.TermsOfService) error
	FindByID(ctx context.Context, id int64) (*model.TermsOfService, error)
	// FindCurrent returns the latest version whose effective_date <= now, or nil if none exists.
	FindCurrent(ctx context.Context) (*model.TermsOfService, error)
	// FindFuture returns all versions whose effective_date > now, ordered by effective_date ASC.
	FindFuture(ctx context.Context) ([]*model.TermsOfService, error)
	// FindAll は規約の全バージョン一覧（管理画面）。窓を取るのは、ここも以前は
	// 引数が無く全件返していたため。版は年に数回しか増えないので実データが
	// 窓に当たることはまず無いが、青天井の口を残さない。
	FindAll(ctx context.Context, q PageQuery) ([]*model.TermsOfService, error)
	SaveConsent(ctx context.Context, c *model.TermsConsent) error
	FindConsent(ctx context.Context, userID, termsID int64) (*model.TermsConsent, error)
	FindConsentsByTermsID(ctx context.Context, termsID int64, q PageQuery) ([]*model.TermsConsent, int, error)
}
