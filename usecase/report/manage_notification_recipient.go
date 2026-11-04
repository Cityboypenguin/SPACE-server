package report

import (
	"context"
	"errors"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ManageNotificationRecipientUsecase は通報のお知らせメールの宛先を管理する（管理者のみ）。
type ManageNotificationRecipientUsecase struct {
	recipients repository.ReportNotificationRecipientRepository
}

func NewManageNotificationRecipientUsecase(recipients repository.ReportNotificationRecipientRepository) *ManageNotificationRecipientUsecase {
	return &ManageNotificationRecipientUsecase{recipients: recipients}
}

func (u *ManageNotificationRecipientUsecase) List(ctx context.Context) ([]*model.ReportNotificationRecipient, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	return u.recipients.List(ctx)
}

func (u *ManageNotificationRecipientUsecase) Add(ctx context.Context, email string) (*model.ReportNotificationRecipient, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	normalized, err := model.NormalizeNotificationEmail(email)
	if err != nil {
		return nil, err
	}
	recipient := &model.ReportNotificationRecipient{Email: normalized, CreatedAt: time.Now()}
	if err := u.recipients.Create(ctx, recipient); err != nil {
		if errors.Is(err, repository.ErrDuplicateKey) {
			return nil, apperr.Conflict("このメールアドレスは登録済みです")
		}
		return nil, err
	}
	return recipient, nil
}

func (u *ManageNotificationRecipientUsecase) Remove(ctx context.Context, id int64) error {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return err
	}
	deleted, err := u.recipients.Delete(ctx, id)
	if err != nil {
		return err
	}
	if !deleted {
		return apperr.NotFound("メールアドレスが見つかりません")
	}
	return nil
}
