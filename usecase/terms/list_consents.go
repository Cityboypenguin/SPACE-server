package terms

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListConsentsUseCase struct {
	TermsRepository repository.TermsRepository
}

func NewListConsentsUseCase(r repository.TermsRepository) *ListConsentsUseCase {
	return &ListConsentsUseCase{TermsRepository: r}
}

func (uc *ListConsentsUseCase) Execute(ctx context.Context, termsID int64, q repository.PageQuery) ([]*model.TermsConsent, int, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}
	return uc.TermsRepository.FindConsentsByTermsID(ctx, termsID, q)
}
