package timetable

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// fakeTimetableRepo records the arguments Upsert was called with. The actual
// "overwrite whatever occupies the same day/period slot" behavior (F-01/F-02) is
// implemented as a single transactional SQL statement in
// infra/mysql/timetable_repository.go, not here, so it is exercised via the live
// dev DB rather than duplicated with a fake; this test only pins the usecase's own
// responsibility: requiring auth and delegating the caller's ID correctly.
type fakeTimetableRepo struct {
	repository.TimetableRepository
	gotUserID, gotCourseID int64
	result                 *model.Timetable
}

func (f *fakeTimetableRepo) Upsert(_ context.Context, userID, courseID int64) (*model.Timetable, error) {
	f.gotUserID, f.gotCourseID = userID, courseID
	return f.result, nil
}

func TestRegisterTimetable_RequiresAuth(t *testing.T) {
	uc := NewRegisterTimetableUseCase(&fakeTimetableRepo{})

	if _, err := uc.Execute(context.Background(), 1); err == nil {
		t.Fatal("expected error when no claims are present in context")
	}
}

func TestRegisterTimetable_DelegatesToRepoWithCallerID(t *testing.T) {
	repo := &fakeTimetableRepo{result: &model.Timetable{ID: 10, UserID: 7, CourseID: 42}}
	uc := NewRegisterTimetableUseCase(repo)
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 7})

	got, err := uc.Execute(ctx, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotUserID != 7 || repo.gotCourseID != 42 {
		t.Fatalf("Upsert called with (%d, %d), want (7, 42)", repo.gotUserID, repo.gotCourseID)
	}
	if got.ID != 10 {
		t.Errorf("returned timetable ID = %d, want 10", got.ID)
	}
}
