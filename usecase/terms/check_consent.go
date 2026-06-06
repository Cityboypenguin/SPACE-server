package terms

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ConsentStatus struct {
	IsConsented  bool
	CurrentTerms *model.TermsOfService
}

type CheckConsentUseCase struct {
	termsRepo repository.TermsRepository
}

func NewCheckConsentUseCase(termsRepo repository.TermsRepository) *CheckConsentUseCase {
	return &CheckConsentUseCase{termsRepo: termsRepo}
}

func (u *CheckConsentUseCase) Execute(ctx context.Context, userID int64) (*ConsentStatus, error) {
	current, err := u.termsRepo.FindCurrent(ctx)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return &ConsentStatus{IsConsented: true, CurrentTerms: nil}, nil
	}

	consent, err := u.termsRepo.FindConsent(ctx, userID, current.ID)
	if err != nil {
		return nil, err
	}

	return &ConsentStatus{
		IsConsented:  consent != nil,
		CurrentTerms: current,
	}, nil
}
