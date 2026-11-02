package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type ReportRepository interface {
	Save(ctx context.Context, report *model.Report) error
	FindByID(ctx context.Context, id string) (*model.Report, error)
	UpdateStatus(ctx context.Context, id string, status model.ReportStatus) (*model.Report, error)
	Search(ctx context.Context, filter *model.ReportSearchFilter, q PageQuery) ([]*model.Report, int, error)
	ReportedMediaRepository
}

// ReportedMediaRepository は通報時点の写しが指している添付を扱う。写しは暗号化してあり
// SQL で中を見られないので、通報の対象で絞ってから読む。
type ReportedMediaRepository interface {
	// HasReportedMedia はその対象の通報の写しのどれかが storageKey を指しているかを返す。
	HasReportedMedia(ctx context.Context, targetType model.ReportTargetType, targetID, storageKey string) (bool, error)
	// RepointReportedMedia はその対象の通報の写しのうち oldKey を指すものを newKey へ付け替える。
	RepointReportedMedia(ctx context.Context, targetType model.ReportTargetType, targetID, oldKey, newKey string) error
}
