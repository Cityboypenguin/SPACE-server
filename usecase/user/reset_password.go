package user

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ResetPasswordUseCase interface {
	Execute(ctx context.Context, resetToken, newPassword string) error
}

var _ ResetPasswordUseCase = &ResetPasswordInteractor{}

type ResetPasswordInteractor struct {
	userRepo    userPasswordRepository
	pwResetRepo repository.PasswordResetRepository
}

func NewResetPasswordUseCase(
	userRepo userPasswordRepository,
	pwResetRepo repository.PasswordResetRepository,
) ResetPasswordUseCase {
	return &ResetPasswordInteractor{
		userRepo:    userRepo,
		pwResetRepo: pwResetRepo,
	}
}

func (uc *ResetPasswordInteractor) Execute(ctx context.Context, resetToken, newPassword string) error {
	if err := model.ValidateUserPassword(newPassword); err != nil {
		return err
	}
	email, err := uc.pwResetRepo.ConsumeResetToken(ctx, resetToken)
	if err != nil {
		return err
	}
	if email == "" {
		return errors.New("invalid or expired reset token")
	}

	// 新しいハッシュを書き込む経路なので認証情報側を取る。
	user, err := uc.userRepo.FindCredentialsByEmail(ctx, email)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}

	if err := user.UpdateUser(model.UpdateUserParam{Password: &newPassword}); err != nil {
		return err
	}

	if err := uc.userRepo.SaveCredentials(ctx, user); err != nil {
		return err
	}

	return nil
}
