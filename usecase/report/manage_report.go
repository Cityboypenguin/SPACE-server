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

func (u *ManageReportUsecase) Search(ctx context.Context, filter *model.ReportSearchFilter) ([]*model.Report, error) {
	return u.reportRepo.Search(ctx, filter)
}

func (u *ManageReportUsecase) UpdateStatus(ctx context.Context, id string, status model.ReportStatus) (*model.Report, error) {
	if id == "" {
		return nil, errors.New("report ID is required")
	}
	return u.reportRepo.UpdateStatus(ctx, id, status)
}

func (u *ManageReportUsecase) ToggleSystem(ctx context.Context, enabled bool) (bool, error) {
	return enabled, nil
}