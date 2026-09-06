package timetable

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeUserSettingRepo struct {
	repository.UserSettingRepository
	value string
	found bool
}

func (f *fakeUserSettingRepo) Get(_ context.Context, _ int64, _ string) (string, bool, error) {
	return f.value, f.found, nil
}

type fakeListTimetableRepo struct {
	repository.TimetableRepository
	gotUserID int64
	result    []*repository.TimetableEntryWithCourse
}

func (f *fakeListTimetableRepo) ListByUser(_ context.Context, userID int64, _ int, _ string) ([]*repository.TimetableEntryWithCourse, error) {
	f.gotUserID = userID
	return f.result, nil
}

func TestGetUserTimetable_RequiresAuth(t *testing.T) {
	uc := NewGetUserTimetableUseCase(&fakeListTimetableRepo{}, nil, &fakeUserSettingRepo{})

	y, s := 2026, "前期"
	if _, err := uc.Execute(context.Background(), 1, &y, &s); err == nil {
		t.Fatal("expected error when no claims are present in context")
	}
}

func TestGetUserTimetable_OwnerSeesOwnHiddenTimetable(t *testing.T) {
	want := []*repository.TimetableEntryWithCourse{{}}
	repo := &fakeListTimetableRepo{result: want}
	uc := NewGetUserTimetableUseCase(repo, nil, &fakeUserSettingRepo{value: "false", found: true})
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 7})

	y, s := 2026, "前期"
	got, err := uc.Execute(ctx, 7, &y, &s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	if repo.gotUserID != 7 {
		t.Fatalf("ListByUser called with userID = %d, want 7", repo.gotUserID)
	}
}

func TestGetUserTimetable_AdminSeesHiddenTimetable(t *testing.T) {
	want := []*repository.TimetableEntryWithCourse{{}}
	repo := &fakeListTimetableRepo{result: want}
	uc := NewGetUserTimetableUseCase(repo, nil, &fakeUserSettingRepo{value: "false", found: true})
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 99, Role: "admin"})

	y, s := 2026, "前期"
	got, err := uc.Execute(ctx, 7, &y, &s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d (admin should bypass visibility)", len(got), len(want))
	}
}

func TestGetUserTimetable_OtherUserHiddenReturnsEmpty(t *testing.T) {
	repo := &fakeListTimetableRepo{result: []*repository.TimetableEntryWithCourse{{}}}
	uc := NewGetUserTimetableUseCase(repo, nil, &fakeUserSettingRepo{value: "false", found: true})
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 1})

	y, s := 2026, "前期"
	got, err := uc.Execute(ctx, 7, &y, &s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d entries, want 0 for a hidden timetable viewed by another user", len(got))
	}
}

func TestGetUserTimetable_IsProfileVisible(t *testing.T) {
	cases := []struct {
		name  string
		value string
		found bool
		want  bool
	}{
		{name: "hidden", value: "false", found: true, want: false},
		{name: "shown", value: "true", found: true, want: true},
		{name: "unset defaults to visible", found: false, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uc := NewGetUserTimetableUseCase(&fakeListTimetableRepo{}, nil, &fakeUserSettingRepo{value: tc.value, found: tc.found})
			got, err := uc.IsProfileVisible(context.Background(), 7)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("IsProfileVisible() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGetUserTimetable_OtherUserDefaultVisibleReturnsEntries(t *testing.T) {
	want := []*repository.TimetableEntryWithCourse{{}}
	repo := &fakeListTimetableRepo{result: want}
	uc := NewGetUserTimetableUseCase(repo, nil, &fakeUserSettingRepo{found: false})
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 1})

	y, s := 2026, "前期"
	got, err := uc.Execute(ctx, 7, &y, &s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d (unset visibility defaults to visible)", len(got), len(want))
	}
}
