package terms

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListConsentsUseCase struct {
	TermsRepository repository.TermsRepository
}

func NewListConsentsUseCase(r repository.TermsRepository) *ListConsentsUseCase {
	return &ListConsentsUseCase{TermsRepository: r}
}

func (uc *ListConsentsUseCase) Execute(ctx context.Context, termsID int64, limit, offset int) ([]*model.TermsConsent, int, error) {
	return uc.TermsRepository.FindConsentsByTermsID(ctx, termsID, limit, offset)
}
