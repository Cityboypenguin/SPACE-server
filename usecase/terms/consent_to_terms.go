package terms

import (
	"context"
	"errors"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ConsentToTermsUseCase struct {
	termsRepo repository.TermsRepository
	userRepo  repository.UserRepository
}

func NewConsentToTermsUseCase(termsRepo repository.TermsRepository, userRepo repository.UserRepository) *ConsentToTermsUseCase {
	return &ConsentToTermsUseCase{termsRepo: termsRepo, userRepo: userRepo}
}

func (u *ConsentToTermsUseCase) Execute(ctx context.Context, userID, termsID int64) error {
	user, err := u.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}

	t, err := u.termsRepo.FindByID(ctx, termsID)
	if err != nil {
		return err
	}
	if t == nil {
		return errors.New("terms not found")
	}

	consent := &model.TermsConsent{
		UserID:      userID,
		TermsID:     termsID,
		ConsentedAt: time.Now(),
	}
	return u.termsRepo.SaveConsent(ctx, consent)
}
