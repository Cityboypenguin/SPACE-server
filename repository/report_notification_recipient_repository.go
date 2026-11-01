package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type ReportNotificationRecipientRepository interface {
	List(ctx context.Context) ([]*model.ReportNotificationRecipient, error)
	// Create は同じアドレスが登録済みなら ErrDuplicateKey を返す。
	Create(ctx context.Context, recipient *model.ReportNotificationRecipient) error
	// Delete は該当の行が無ければ false を返す。
	Delete(ctx context.Context, id int64) (bool, error)
}
