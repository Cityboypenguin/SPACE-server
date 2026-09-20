package user

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type signupOTPRepo struct {
	repository.EmailOTPRepository
	mu       sync.Mutex
	reserved bool
	saved    bool
	code     string
	deletes  int
	saveErr  error
}

func (r *signupOTPRepo) TryBeginSend(context.Context, string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reserved {
		return false, nil
	}
	r.reserved = true
	return true, nil
}

func (r *signupOTPRepo) Save(_ context.Context, otp *model.EmailOTP) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.saved = true
	r.code = otp.Code
	return nil
}

func (r *signupOTPRepo) FindLatestByEmail(context.Context, string) (*model.EmailOTP, error) {
	return &model.EmailOTP{Code: r.code}, nil
}

func (r *signupOTPRepo) Delete(context.Context, string) error {
	r.deletes++
	return nil
}

type checkingMailer struct {
	repo *signupOTPRepo
	sent int
}

func (m *checkingMailer) Send(context.Context, string, string, string) error {
	if !m.repo.saved {
		return errors.New("mail was sent before OTP was stored")
	}
	m.sent++
	return nil
}

func TestSignupOTPSavedBeforeMailAndRecipientRateLimited(t *testing.T) {
	ctx := context.Background()
	repo := &signupOTPRepo{}
	mailer := &checkingMailer{repo: repo}
	uc := NewSendEmailOTPUseCase(repo, &fakeUserRepo{}, mailer)
	if err := uc.Execute(ctx, "EE201234@senshu-u.jp"); err != nil {
		t.Fatal(err)
	}
	if err := uc.Execute(ctx, "EE201234@senshu-u.jp"); err == nil {
		t.Fatal("second send was not rate limited")
	}
	if mailer.sent != 1 {
		t.Fatalf("sent %d mails, want 1", mailer.sent)
	}
	failedRepo := &signupOTPRepo{saveErr: errors.New("redis unavailable")}
	failedMailer := &checkingMailer{repo: failedRepo}
	if err := NewSendEmailOTPUseCase(failedRepo, &fakeUserRepo{}, failedMailer).Execute(ctx, "EE201234@senshu-u.jp"); err == nil || failedMailer.sent != 0 {
		t.Fatalf("save failure sent mail: sent=%d err=%v", failedMailer.sent, err)
	}
}

type resetOTPRepo struct {
	repository.PasswordResetRepository
	mu      sync.Mutex
	usedOTP bool
	usedKey bool
	saved   int
}

func (r *resetOTPRepo) ExchangeOTPForResetToken(_ context.Context, _, otp, _ string, _ time.Duration) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.usedOTP || otp != "123456" {
		return false, nil
	}
	r.usedOTP = true
	r.saved++
	return true, nil
}

func (r *resetOTPRepo) ConsumeResetToken(context.Context, string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.usedKey {
		return "", nil
	}
	r.usedKey = true
	return "EE201234@senshu-u.jp", nil
}

func TestResetOTPAndTokenAreSingleUse(t *testing.T) {
	repo := &resetOTPRepo{}
	verify := NewVerifyPasswordResetOTPUseCase(repo)
	var wg sync.WaitGroup
	results := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, _ := verify.Execute(context.Background(), "EE201234@senshu-u.jp", "123456")
			results <- token
		}()
	}
	wg.Wait()
	close(results)
	var issued int
	for token := range results {
		if token != "" {
			issued++
		}
	}
	if issued != 1 || repo.saved != 1 {
		t.Fatalf("issued=%d saved=%d, want one reset token", issued, repo.saved)
	}
	if email, err := repo.ConsumeResetToken(context.Background(), "reset-token"); err != nil || email == "" {
		t.Fatalf("first token consume: email=%q err=%v", email, err)
	}
	if email, err := repo.ConsumeResetToken(context.Background(), "reset-token"); err != nil || email != "" {
		t.Fatalf("second token consume: email=%q err=%v", email, err)
	}
}

type signupUserRepo struct{ repository.UserRepository }

func (*signupUserRepo) SaveCredentials(_ context.Context, credentials *model.UserCredentials) error {
	credentials.ID = 1
	return nil
}

type signupProfileRepo struct{ repository.ProfileRepository }

func (*signupProfileRepo) SaveProfile(context.Context, *model.Profile) error { return nil }

type signupTx struct{ commitErr error }

func (t signupTx) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	if err := fn(ctx); err != nil {
		return err
	}
	return t.commitErr
}

func TestRegistrationOTPIsDeletedOnlyAfterCommit(t *testing.T) {
	t.Setenv("DISABLE_USER_VALIDATION", "false")
	param := model.CreateUserParam{
		AccountID: "testuser", Name: "Test", Email: "EE201234@senshu-u.jp", Password: "password123",
	}
	failedOTP := &signupOTPRepo{code: "123456"}
	failed := NewCreateUserUseCase(&signupUserRepo{}, &signupProfileRepo{}, failedOTP, signupTx{commitErr: errors.New("commit failed")})
	if _, err := failed.Execute(context.Background(), param, "123456"); err == nil {
		t.Fatal("failed commit was reported as success")
	}
	if failedOTP.deletes != 0 {
		t.Fatal("registration OTP was deleted before commit")
	}
	goodOTP := &signupOTPRepo{code: "123456"}
	good := NewCreateUserUseCase(&signupUserRepo{}, &signupProfileRepo{}, goodOTP, signupTx{})
	if _, err := good.Execute(context.Background(), param, "123456"); err != nil {
		t.Fatal(err)
	}
	if goodOTP.deletes != 1 {
		t.Fatalf("successful registration deleted OTP %d times", goodOTP.deletes)
	}
}
