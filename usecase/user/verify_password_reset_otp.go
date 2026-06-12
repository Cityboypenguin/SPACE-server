package user

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

const resetTokenTTL = 15 * time.Minute

type VerifyPasswordResetOTPUseCase interface {
	Execute(ctx context.Context, email, otp string) (string, error)
}

var _ VerifyPasswordResetOTPUseCase = &VerifyPasswordResetOTPInteractor{}

type VerifyPasswordResetOTPInteractor struct {
	pwResetRepo repository.PasswordResetRepository
}

func NewVerifyPasswordResetOTPUseCase(pwResetRepo repository.PasswordResetRepository) VerifyPasswordResetOTPUseCase {
	return &VerifyPasswordResetOTPInteractor{pwResetRepo: pwResetRepo}
}

func (uc *VerifyPasswordResetOTPInteractor) Execute(ctx context.Context, email, otp string) (string, error) {
	stored, err := uc.pwResetRepo.GetOTP(ctx, email)
	if err != nil {
		return "", err
	}
	if stored == "" || stored != otp {
		return "", errors.New("invalid or expired verification code")
	}

	if err := uc.pwResetRepo.DeleteOTP(ctx, email); err != nil {
		return "", err
	}

	token, err := generateResetToken()
	if err != nil {
		return "", err
	}

	if err := uc.pwResetRepo.SaveResetToken(ctx, token, email, resetTokenTTL); err != nil {
		return "", err
	}

	return token, nil
}

func generateResetToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
