package report

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/google/uuid"
)

type CreateReportUsecase struct {
	reportRepo  repository.ReportRepository
	settingRepo repository.SystemSettingRepository
}

func NewCreateReportUsecase(reportRepo repository.ReportRepository, settingRepo repository.SystemSettingRepository) *CreateReportUsecase {
	return &CreateReportUsecase{
		reportRepo:  reportRepo,
		settingRepo: settingRepo,
	}
}

type CreateReportInput struct {
	ReporterID   int64
	TargetType   model.ReportTargetType
	TargetID     string
	Reason       string
	CustomReason *string
	Content      *string
}

func (u *CreateReportUsecase) Execute(ctx context.Context, input CreateReportInput) (*model.Report, error) {
	isEnabled, err := u.settingRepo.GetBool(ctx, "is_report_enabled")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch system setting: %w", err)
	}

	if !isEnabled {
		return nil, errors.New("現在、システム全体の通報機能は一時的に停止されています")
	}

	if input.TargetID == "" || input.Reason == "" {
		return nil, errors.New("invalid report input: targetID and reason are required")
	}

	if input.TargetType == model.TargetUser && input.TargetID == strconv.FormatInt(input.ReporterID, 10) {
		return nil, errors.New("自分自身を通報することはできません")
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
