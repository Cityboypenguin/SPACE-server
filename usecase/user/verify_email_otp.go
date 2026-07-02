package user

import (
	"context"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type VerifyEmailOTPUseCase interface {
	Execute(ctx context.Context, email, otp string) error
}

var _ VerifyEmailOTPUseCase = &VerifyEmailOTPInteractor{}

type VerifyEmailOTPInteractor struct {
	otpRepo repository.EmailOTPRepository
}

func NewVerifyEmailOTPUseCase(otpRepo repository.EmailOTPRepository) VerifyEmailOTPUseCase {
	return &VerifyEmailOTPInteractor{otpRepo: otpRepo}
}

func (uc *VerifyEmailOTPInteractor) Execute(ctx context.Context, email, otp string) error {
	if err := model.ValidateUserEmail(email); err != nil {
		return err
	}
	if len(otp) != 6 {
		return fmt.Errorf("認証コードは6桁で入力してください")
	}
	if otp == "" {
		return fmt.Errorf("認証コードが無効または期限切れです")
	}

	record, err := uc.otpRepo.FindLatestByEmail(ctx, email)
	if err != nil {
		return err
	}
	if record == nil || record.Code != otp {
		return fmt.Errorf("認証コードが無効または期限切れです")
	}
	return nil
}
