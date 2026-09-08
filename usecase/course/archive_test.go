package course

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeCourseRepoForArchive struct {
	repository.CourseRepository
	course *model.Course
}

func (f *fakeCourseRepoForArchive) GetCourseByRoomID(_ context.Context, _ int64) (*model.Course, error) {
	return f.course, nil
}

type fakeSettingRepo struct {
	repository.SystemSettingRepository
	currentSemester string
}

func (f *fakeSettingRepo) Get(_ context.Context, _ string) (string, error) {
	return f.currentSemester, nil
}

type fakeTimetableRepoForArchive struct {
	repository.TimetableRepository
	registered bool
}

func (f *fakeTimetableRepoForArchive) IsRegistered(_ context.Context, _, _ int64) (bool, error) {
	return f.registered, nil
}

func ctxAsStudent() context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: 42, Role: "user"})
}

func TestCheckRoomWritable_NonCourseRoomAlwaysWritable(t *testing.T) {
	uc := NewCheckRoomWritableUseCase(&fakeCourseRepoForArchive{course: nil}, &fakeSettingRepo{currentSemester: "2026:前期"}, &fakeTimetableRepoForArchive{registered: false})

	if err := uc.Execute(context.Background(), 1); err != nil {
		t.Fatalf("non-course rooms must never be archived, got: %v", err)
	}
}

func TestCheckRoomWritable_CurrentSemesterIsWritable(t *testing.T) {
	repo := &fakeCourseRepoForArchive{course: &model.Course{RoomID: 1, Year: 2026, Semester: model.SemesterFirst}}
	uc := NewCheckRoomWritableUseCase(repo, &fakeSettingRepo{currentSemester: "2026:前期"}, &fakeTimetableRepoForArchive{registered: true})

	if err := uc.Execute(ctxAsStudent(), 1); err != nil {
		t.Fatalf("course room matching the current semester and registered by the caller must be writable, got: %v", err)
	}
}

func TestCheckRoomWritable_PastSemesterIsForbidden(t *testing.T) {
	repo := &fakeCourseRepoForArchive{course: &model.Course{RoomID: 1, Year: 2025, Semester: model.SemesterSecond}}
	uc := NewCheckRoomWritableUseCase(repo, &fakeSettingRepo{currentSemester: "2026:前期"}, &fakeTimetableRepoForArchive{registered: true})

	err := uc.Execute(ctxAsStudent(), 1)
	if err == nil {
		t.Fatal("expected an archive rejection for a past-semester course room")
	}
	if apperr.CodeOf(err) != apperr.CodeForbidden {
		t.Fatalf("error code = %s, want %s", apperr.CodeOf(err), apperr.CodeForbidden)
	}
}

func TestCheckRoomWritable_FullYearCourseIsWritableInEitherSemester(t *testing.T) {
	repo := &fakeCourseRepoForArchive{course: &model.Course{RoomID: 1, Year: 2026, Semester: model.SemesterFull}}

	for _, current := range []string{"前期", "後期"} {
		uc := NewCheckRoomWritableUseCase(repo, &fakeSettingRepo{currentSemester: "2026:" + current}, &fakeTimetableRepoForArchive{registered: true})
		if err := uc.Execute(ctxAsStudent(), 1); err != nil {
			t.Fatalf("通年 course must be writable in %s of its own year, got: %v", current, err)
		}
	}
}

func TestCheckRoomWritable_FullYearCourseIsArchivedNextYear(t *testing.T) {
	repo := &fakeCourseRepoForArchive{course: &model.Course{RoomID: 1, Year: 2025, Semester: model.SemesterFull}}
	uc := NewCheckRoomWritableUseCase(repo, &fakeSettingRepo{currentSemester: "2026:前期"}, &fakeTimetableRepoForArchive{registered: true})

	err := uc.Execute(ctxAsStudent(), 1)
	if err == nil {
		t.Fatal("expected an archive rejection for a 通年 course from a past academic year")
	}
	if apperr.CodeOf(err) != apperr.CodeForbidden {
		t.Fatalf("error code = %s, want %s", apperr.CodeOf(err), apperr.CodeForbidden)
	}
}

func TestCheckRoomWritable_NotRegisteredIsForbidden(t *testing.T) {
	repo := &fakeCourseRepoForArchive{course: &model.Course{RoomID: 1, Year: 2026, Semester: model.SemesterFirst}}
	uc := NewCheckRoomWritableUseCase(repo, &fakeSettingRepo{currentSemester: "2026:前期"}, &fakeTimetableRepoForArchive{registered: false})

	err := uc.Execute(ctxAsStudent(), 1)
	if err == nil {
		t.Fatal("expected a rejection for a caller who hasn't registered for the course")
	}
	if apperr.CodeOf(err) != apperr.CodeForbidden {
		t.Fatalf("error code = %s, want %s", apperr.CodeOf(err), apperr.CodeForbidden)
	}
}

func TestRequireWritableCourseRoom_RejectsNonCourseRoom(t *testing.T) {
	uc := NewRequireWritableCourseRoomUseCase(&fakeCourseRepoForArchive{course: nil}, &fakeSettingRepo{currentSemester: "2026:前期"}, &fakeTimetableRepoForArchive{registered: false})

	_, err := uc.Execute(context.Background(), 1)
	if err == nil {
		t.Fatal("expected an error for a room that isn't a course room at all")
	}
	if apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("error code = %s, want %s", apperr.CodeOf(err), apperr.CodeInvalidInput)
	}
}

func TestRequireWritableCourseRoom_ReturnsCourseWhenWritable(t *testing.T) {
	want := &model.Course{ID: 9, RoomID: 1, Year: 2026, Semester: model.SemesterFirst}
	uc := NewRequireWritableCourseRoomUseCase(&fakeCourseRepoForArchive{course: want}, &fakeSettingRepo{currentSemester: "2026:前期"}, &fakeTimetableRepoForArchive{registered: true})

	got, err := uc.Execute(ctxAsStudent(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("course ID = %d, want %d", got.ID, want.ID)
	}
}

func TestRequireWritableCourseRoom_PastSemesterIsForbidden(t *testing.T) {
	repo := &fakeCourseRepoForArchive{course: &model.Course{RoomID: 1, Year: 2025, Semester: model.SemesterFirst}}
	uc := NewRequireWritableCourseRoomUseCase(repo, &fakeSettingRepo{currentSemester: "2026:前期"}, &fakeTimetableRepoForArchive{registered: true})

	_, err := uc.Execute(ctxAsStudent(), 1)
	if apperr.CodeOf(err) != apperr.CodeForbidden {
		t.Fatalf("error code = %s, want %s", apperr.CodeOf(err), apperr.CodeForbidden)
	}
}

func TestRequireWritableCourseRoom_FullYearCourseIsWritableInEitherSemester(t *testing.T) {
	repo := &fakeCourseRepoForArchive{course: &model.Course{ID: 9, RoomID: 1, Year: 2026, Semester: model.SemesterFull}}

	for _, current := range []string{"前期", "後期"} {
		uc := NewRequireWritableCourseRoomUseCase(repo, &fakeSettingRepo{currentSemester: "2026:" + current}, &fakeTimetableRepoForArchive{registered: true})
		if _, err := uc.Execute(ctxAsStudent(), 1); err != nil {
			t.Fatalf("通年 course must be writable in %s of its own year, got: %v", current, err)
		}
	}
}

func TestRequireWritableCourseRoom_NotRegisteredIsForbidden(t *testing.T) {
	repo := &fakeCourseRepoForArchive{course: &model.Course{RoomID: 1, Year: 2026, Semester: model.SemesterFirst}}
	uc := NewRequireWritableCourseRoomUseCase(repo, &fakeSettingRepo{currentSemester: "2026:前期"}, &fakeTimetableRepoForArchive{registered: false})

	_, err := uc.Execute(ctxAsStudent(), 1)
	if err == nil {
		t.Fatal("expected a rejection for a caller who hasn't registered for the course")
	}
	if apperr.CodeOf(err) != apperr.CodeForbidden {
		t.Fatalf("error code = %s, want %s", apperr.CodeOf(err), apperr.CodeForbidden)
	}
}
