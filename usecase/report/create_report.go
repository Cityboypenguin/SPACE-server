package report

import (
	"context"
	"errors"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/google/uuid"
)

type CreateReportUsecase struct {
	reportRepo repository.ReportRepository
}

func NewCreateReportUsecase(reportRepo repository.ReportRepository) *CreateReportUsecase {
	return &CreateReportUsecase{reportRepo: reportRepo}
}

type CreateReportInput struct {
	ReporterID   int64
	TargetType   model.ReportTargetType
	TargetID     string
	Reason       string
	CustomReason *string
}

func (u *CreateReportUsecase) Execute(ctx context.Context, input CreateReportInput) (*model.Report, error) {
	if input.TargetID == "" || input.Reason == "" {
		return nil, errors.New("invalid report input: targetID and reason are required")
	}

	now := time.Now()
	report := &model.Report{
		ID:           uuid.New().String(),
		ReporterID:   input.ReporterID,
		TargetType:   input.TargetType,
		TargetID:     input.TargetID,
		Reason:       input.Reason,
		CustomReason: input.CustomReason,
		Status:       model.StatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := u.reportRepo.Save(ctx, report); err != nil {
		return nil, err
	}

	return report, nil
}