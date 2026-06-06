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
	FindAll(ctx context.Context) ([]*model.TermsOfService, error)
	SaveConsent(ctx context.Context, c *model.TermsConsent) error
	FindConsent(ctx context.Context, userID, termsID int64) (*model.TermsConsent, error)
	FindConsentsByTermsID(ctx context.Context, termsID int64) ([]*model.TermsConsent, error)
}
