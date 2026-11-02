package user

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/async"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

const otpTTL = 10 * time.Minute
const resetRequestInterval = time.Minute

type RequestPasswordResetUseCase interface {
	Execute(ctx context.Context, email string) error
}

var _ RequestPasswordResetUseCase = &RequestPasswordResetInteractor{}

type RequestPasswordResetInteractor struct {
	userRepo    repository.UserReader
	pwResetRepo repository.PasswordResetRepository
	mailer      repository.Mailer
	// mailRunner はメールをリクエストの外で送る。nil ならその場で送る（テスト用）。
	mailRunner *async.Runner
}

func NewRequestPasswordResetUseCase(
	userRepo repository.UserReader,
	pwResetRepo repository.PasswordResetRepository,
	mailer repository.Mailer,
	mailRunner *async.Runner,
) RequestPasswordResetUseCase {
	return &RequestPasswordResetInteractor{
		userRepo:    userRepo,
		pwResetRepo: pwResetRepo,
		mailer:      mailer,
		mailRunner:  mailRunner,
	}
}

// Execute は登録済みのアドレスにだけ認証コードを送る。未登録のアドレスには何も送らない。
//
// 応答からアドレスの登録の有無が分からないようにする。どちらの場合も同じ成功を返し、
// メールはリクエストの外で送るので、送信にかかる時間も送信の失敗も応答に出ない。
// 送れなかったときはログに残す（利用者には、届かなければもう一度頼んでもらう）。
func (uc *RequestPasswordResetInteractor) Execute(ctx context.Context, email string) error {
	allowed, err := uc.pwResetRepo.TryBeginRequest(ctx, email, resetRequestInterval)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
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

	// 件名・本文はこの用途の文言なのでユースケース側に置く。
	// Mailer は「1通送る」だけを知っていればよい。
	subject := "パスワードリセット認証コード"
	body := fmt.Sprintf("認証コード: %s\n\nこのコードは10分間有効です。\n心当たりがない場合は無視してください。", otp)
	send := func(ctx context.Context) {
		if err := uc.mailer.Send(ctx, email, subject, body); err != nil {
			logger.Log.Error().Err(err).Int64("user_id", user.ID).Msg("failed to send the password reset code")
		}
	}
	if uc.mailRunner == nil {
		send(ctx)
		return nil
	}
	uc.mailRunner.Go(ctx, "password_reset_mail", send)
	return nil
}

func generateOTP() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	n := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
	return fmt.Sprintf("%06d", n%1000000), nil
}
