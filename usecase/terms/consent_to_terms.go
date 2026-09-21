package terms

import (
	"context"

	"errors"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ConsentToTermsUseCase struct {
	termsRepo repository.TermsRepository
	userRepo  repository.UserReader
}

func NewConsentToTermsUseCase(termsRepo repository.TermsRepository, userRepo repository.UserReader) *ConsentToTermsUseCase {
	return &ConsentToTermsUseCase{termsRepo: termsRepo, userRepo: userRepo}
}

func (u *ConsentToTermsUseCase) Execute(ctx context.Context, termsID int64) error {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return err
	}
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
