package user

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

const otpTTL = 10 * time.Minute

type RequestPasswordResetUseCase interface {
	Execute(ctx context.Context, email string) error
}

var _ RequestPasswordResetUseCase = &RequestPasswordResetInteractor{}

type RequestPasswordResetInteractor struct {
	userRepo      repository.UserRepository
	pwResetRepo   repository.PasswordResetRepository
	mailer        repository.Mailer
}

func NewRequestPasswordResetUseCase(
	userRepo repository.UserRepository,
	pwResetRepo repository.PasswordResetRepository,
	mailer repository.Mailer,
) RequestPasswordResetUseCase {
	return &RequestPasswordResetInteractor{
		userRepo:    userRepo,
		pwResetRepo: pwResetRepo,
		mailer:      mailer,
	}
}

func (uc *RequestPasswordResetInteractor) Execute(ctx context.Context, email string) error {
	user, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return err
	}
	if user == nil {
		// メールが登録されていない場合でも成功を返す（ユーザー列挙防止）
		return nil
	}

	otp, err := generateOTP()
	if err != nil {
		return err
	}

	if err := uc.pwResetRepo.SaveOTP(ctx, email, otp, otpTTL); err != nil {
		return err
	}

	return uc.mailer.SendPasswordResetOTP(ctx, email, otp)
}

func generateOTP() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	n := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
	return fmt.Sprintf("%06d", n%1000000), nil
}
