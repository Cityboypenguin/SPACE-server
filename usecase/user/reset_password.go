package user

import (
	"context"
	"errors"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// refreshTokenTTL はリフレッシュトークンの最大有効期間（既存トークンの失効判定に使う）
const passwordChangedAtTTL = 30 * 24 * time.Hour

type ResetPasswordUseCase interface {
	Execute(ctx context.Context, resetToken, newPassword string) error
}

var _ ResetPasswordUseCase = &ResetPasswordInteractor{}

type ResetPasswordInteractor struct {
	userRepo    repository.UserRepository
	pwResetRepo repository.PasswordResetRepository
}

func NewResetPasswordUseCase(
	userRepo repository.UserRepository,
	pwResetRepo repository.PasswordResetRepository,
) ResetPasswordUseCase {
	return &ResetPasswordInteractor{
		userRepo:    userRepo,
		pwResetRepo: pwResetRepo,
	}
}

func (uc *ResetPasswordInteractor) Execute(ctx context.Context, resetToken, newPassword string) error {
	email, err := uc.pwResetRepo.GetEmailByResetToken(ctx, resetToken)
	if err != nil {
		return err
	}
	if email == "" {
		return errors.New("invalid or expired reset token")
	}

	user, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}

	if err := user.UpdateUser(model.UpdateUserParam{Password: &newPassword}); err != nil {
		return err
	}

	if err := uc.userRepo.UpdateUser(ctx, user); err != nil {
		return err
	}

	// リセットトークンを削除
	if err := uc.pwResetRepo.DeleteResetToken(ctx, resetToken); err != nil {
		return err
	}

	// パスワード変更時刻を記録（既存セッションの失効に使用）
	return uc.pwResetRepo.SetPasswordChangedAt(ctx, user.ID, time.Now(), passwordChangedAtTTL)
}
