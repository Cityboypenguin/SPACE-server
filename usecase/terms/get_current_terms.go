package terms

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetCurrentTermsUseCase struct {
	termsRepo repository.TermsRepository
}

func NewGetCurrentTermsUseCase(termsRepo repository.TermsRepository) *GetCurrentTermsUseCase {
	return &GetCurrentTermsUseCase{termsRepo: termsRepo}
}

func (u *GetCurrentTermsUseCase) Execute(ctx context.Context) (*model.TermsOfService, error) {
	return u.termsRepo.FindCurrent(ctx)
}
