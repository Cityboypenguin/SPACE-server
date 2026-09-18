package user

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// SMTP の実装は infra/smtp の1本に統合し、ユースケースは repository.Mailer の
// 汎用 Send（宛先・件名・本文）だけを見る。件名と本文は用途ごとの文言なので
// ユースケース側にあることを、この fake で確認する。
type fakeMailer struct {
	sent []sentMail
	err  error
}

type sentMail struct {
	to      string
	subject string
	body    string
}

func (m *fakeMailer) Send(_ context.Context, to, subject, body string) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, sentMail{to: to, subject: subject, body: body})
	return nil
}

type fakeUserRepo struct {
	repository.UserRepository
	user *model.User
}

func (f *fakeUserRepo) FindByEmail(_ context.Context, _ string) (*model.User, error) {
	return f.user, nil
}

type fakePasswordResetRepo struct {
	repository.PasswordResetRepository
	savedOTP string
}

func (f *fakePasswordResetRepo) SaveOTP(_ context.Context, _, otp string, _ time.Duration) error {
	f.savedOTP = otp
	return nil
}

func TestRequestPasswordReset_SendsOTPThroughMailer(t *testing.T) {
	mailer := &fakeMailer{}
	pwRepo := &fakePasswordResetRepo{}
	uc := NewRequestPasswordResetUseCase(&fakeUserRepo{user: &model.User{ID: 1}}, pwRepo, mailer)

	if err := uc.Execute(context.Background(), "u@example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mailer.sent) != 1 {
		t.Fatalf("sent %d mails, want 1", len(mailer.sent))
	}
	got := mailer.sent[0]
	if got.to != "u@example.com" {
		t.Errorf("to = %q, want u@example.com", got.to)
	}
	if got.subject != "パスワードリセット認証コード" {
		t.Errorf("subject = %q, want the password reset subject", got.subject)
	}
	if pwRepo.savedOTP == "" || !strings.Contains(got.body, pwRepo.savedOTP) {
		t.Errorf("body %q must carry the saved OTP %q", got.body, pwRepo.savedOTP)
	}
}

// 未登録のメールアドレスにはユーザー列挙を避けるため何も送らない（従来どおり）。
func TestRequestPasswordReset_UnknownEmailSendsNothing(t *testing.T) {
	mailer := &fakeMailer{}
	uc := NewRequestPasswordResetUseCase(&fakeUserRepo{}, &fakePasswordResetRepo{}, mailer)

	if err := uc.Execute(context.Background(), "nobody@example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mailer.sent) != 0 {
		t.Fatalf("sent %d mails, want none for an unknown address", len(mailer.sent))
	}
}
