package user

import (
	"context"
	"fmt"
	"os"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type RequestPasswordResetUseCase interface {
	Execute(ctx context.Context, email string) error
}

var _ RequestPasswordResetUseCase = &RequestPasswordResetInteractor{}

type RequestPasswordResetInteractor struct {
	userRepo    repository.UserRepository
	pwResetRepo repository.PasswordResetRepository
	mailer      repository.Mailer
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

	token, err := generateResetToken()
	if err != nil {
		return err
	}

	if err := uc.pwResetRepo.SaveResetToken(ctx, token, email, resetTokenTTL); err != nil {
		return err
	}

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:5173"
	}
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", frontendURL, token)
	return uc.mailer.SendPasswordResetLink(ctx, email, resetLink)
}
