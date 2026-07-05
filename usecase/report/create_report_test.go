package report

import (
	"context"
	"errors"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// fakeReportRepo records the last saved report and satisfies ReportRepository.
// Only Save is exercised here; the other methods are unused by CreateReportUsecase.
type fakeReportRepo struct {
	repository.ReportRepository
	saved   *model.Report
	saveErr error
}

func (f *fakeReportRepo) Save(_ context.Context, r *model.Report) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = r
	return nil
}

type fakeSettingRepo struct {
	repository.SystemSettingRepository
	enabled bool
	err     error
}

func (f *fakeSettingRepo) GetBool(_ context.Context, _ string) (bool, error) {
	return f.enabled, f.err
}

func newUsecase(reportRepo *fakeReportRepo, enabled bool) *CreateReportUsecase {
	return NewCreateReportUsecase(reportRepo, &fakeSettingRepo{enabled: enabled})
}

func TestCreateReport_RejectedWhenDisabled(t *testing.T) {
	repo := &fakeReportRepo{}
	_, err := newUsecase(repo, false).Execute(context.Background(), CreateReportInput{
		ReporterID: 1,
		TargetType: model.TargetPost,
		TargetID:   "42",
		Reason:     "spam",
	})
	if err == nil {
		t.Fatal("expected error when report feature disabled, got nil")
	}
	if repo.saved != nil {
		t.Fatal("report must not be saved when feature disabled")
	}
}

func TestCreateReport_RequiresTargetAndReason(t *testing.T) {
	cases := []struct {
		name  string
		input CreateReportInput
	}{
		{"missing target", CreateReportInput{ReporterID: 1, TargetType: model.TargetPost, TargetID: "", Reason: "spam"}},
		{"missing reason", CreateReportInput{ReporterID: 1, TargetType: model.TargetPost, TargetID: "42", Reason: ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeReportRepo{}
			if _, err := newUsecase(repo, true).Execute(context.Background(), tc.input); err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

func TestCreateReport_RejectsSelfReport(t *testing.T) {
	repo := &fakeReportRepo{}
	_, err := newUsecase(repo, true).Execute(context.Background(), CreateReportInput{
		ReporterID: 7,
		TargetType: model.TargetUser,
		TargetID:   "7",
		Reason:     "abuse",
	})
	if err == nil {
		t.Fatal("expected error reporting oneself, got nil")
	}
}

func TestCreateReport_SavesPendingReport(t *testing.T) {
	repo := &fakeReportRepo{}
	content := "offending post body"
	got, err := newUsecase(repo, true).Execute(context.Background(), CreateReportInput{
		ReporterID: 7,
		TargetType: model.TargetPost,
		TargetID:   "42",
		Reason:     "spam",
		Content:    &content,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.saved == nil {
		t.Fatal("expected report to be saved")
	}
	if got.Status != model.StatusPending {
		t.Errorf("status = %q, want %q", got.Status, model.StatusPending)
	}
	if got.PostContent == nil || *got.PostContent != content {
		t.Errorf("post content snapshot not persisted: %v", got.PostContent)
	}
	if got.ID == "" {
		t.Error("expected a generated report ID")
	}
}

func TestCreateReport_PropagatesSaveError(t *testing.T) {
	repo := &fakeReportRepo{saveErr: errors.New("db down")}
	if _, err := newUsecase(repo, true).Execute(context.Background(), CreateReportInput{
		ReporterID: 1,
		TargetType: model.TargetPost,
		TargetID:   "42",
		Reason:     "spam",
	}); err == nil {
		t.Fatal("expected save error to propagate")
	}
}
