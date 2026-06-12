package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type ReportRepository interface {
	Save(ctx context.Context, report *model.Report) error
	FindByID(ctx context.Context, id string) (*model.Report, error)
	UpdateStatus(ctx context.Context, id string, status model.ReportStatus) (*model.Report, error)
	Search(ctx context.Context, filter *model.ReportSearchFilter, limit, offset int) ([]*model.Report, int, error)
}