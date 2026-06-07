package user

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/Cityboypenguin/SPACE-server/infra/email"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SendEmailOTPUseCase interface {
	Execute(ctx context.Context, emailAddr string) error
}

var _ SendEmailOTPUseCase = &SendEmailOTPInteractor{}

type SendEmailOTPInteractor struct {
	otpRepo      repository.EmailOTPRepository
	emailService email.EmailService
}

func NewSendEmailOTPUseCase(otpRepo repository.EmailOTPRepository, emailService email.EmailService) SendEmailOTPUseCase {
	return &SendEmailOTPInteractor{
		otpRepo:      otpRepo,
		emailService: emailService,
	}
}

func (uc *SendEmailOTPInteractor) Execute(ctx context.Context, emailAddr string) error {
	code, err := generateOTPCode()
	if err != nil {
		return err
	}

	otp := &model.EmailOTP{
		Email:     emailAddr,
		Code:      code,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	if err := uc.otpRepo.Save(ctx, otp); err != nil {
		return err
	}

	subject := "【Senashu-Universe】メールアドレス確認コード"
	body := fmt.Sprintf(
		"Senashu-Universeへの新規登録の確認コードは以下の通りです。\n\n確認コード: %s\n\nこのコードは10分間有効です。\n※このメールに心当たりがない場合は、そのまま削除してください。",
		code,
	)
	return uc.emailService.Send(emailAddr, subject, body)
}

func generateOTPCode() (string, error) {
	var code string
	for i := 0; i < 6; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("failed to generate OTP: %w", err)
		}
		code += n.String()
	}
	return code, nil
}
