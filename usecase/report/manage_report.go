package report

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ManageReportUsecase struct {
	reportRepo repository.ReportRepository
}

func NewManageReportUsecase(reportRepo repository.ReportRepository) *ManageReportUsecase {
	return &ManageReportUsecase{reportRepo: reportRepo}
}

func (u *ManageReportUsecase) Search(ctx context.Context, filter *model.ReportSearchFilter, limit, offset int) ([]*model.Report, int, error) {
	return u.reportRepo.Search(ctx, filter, limit, offset)
}

func (u *ManageReportUsecase) UpdateStatus(ctx context.Context, id string, status model.ReportStatus) (*model.Report, error) {
	if id == "" {
		return nil, errors.New("report ID is required")
	}
	return u.reportRepo.UpdateStatus(ctx, id, status)
}
