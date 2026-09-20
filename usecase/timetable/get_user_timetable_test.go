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

type fakeBlockRepo struct {
	repository.BlockerRepository
	blocked bool
}

func (f *fakeBlockRepo) ExistsBlockRelation(_ context.Context, _, _ int64) (bool, error) {
	return f.blocked, nil
}

func TestGetUserTimetable_RequiresAuth(t *testing.T) {
	uc := NewGetUserTimetableUseCase(&fakeListTimetableRepo{}, nil, &fakeUserSettingRepo{}, &fakeBlockRepo{})

	y, s := 2026, "前期"
	if _, _, err := uc.Execute(context.Background(), 1, &y, &s); err == nil {
		t.Fatal("expected error when no claims are present in context")
	}
}

func TestGetUserTimetable_OwnerSeesOwnHiddenTimetable(t *testing.T) {
	want := []*repository.TimetableEntryWithCourse{{}}
	repo := &fakeListTimetableRepo{result: want}
	uc := NewGetUserTimetableUseCase(repo, nil, &fakeUserSettingRepo{value: "false", found: true}, &fakeBlockRepo{})
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 7})

	y, s := 2026, "前期"
	got, visible, err := uc.Execute(ctx, 7, &y, &s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !visible {
		t.Fatal("visible = false, want true for the owner")
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
	uc := NewGetUserTimetableUseCase(repo, nil, &fakeUserSettingRepo{value: "false", found: true}, &fakeBlockRepo{blocked: true})
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 99, Role: "admin"})

	y, s := 2026, "前期"
	got, visible, err := uc.Execute(ctx, 7, &y, &s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !visible {
		t.Fatal("visible = false, want true for an admin")
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d (admin should bypass visibility)", len(got), len(want))
	}
}

func TestGetUserTimetable_OtherUserHiddenReturnsEmpty(t *testing.T) {
	repo := &fakeListTimetableRepo{result: []*repository.TimetableEntryWithCourse{{}}}
	uc := NewGetUserTimetableUseCase(repo, nil, &fakeUserSettingRepo{value: "false", found: true}, &fakeBlockRepo{})
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 1})

	y, s := 2026, "前期"
	got, visible, err := uc.Execute(ctx, 7, &y, &s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if visible {
		t.Fatal("visible = true, want false for a hidden timetable viewed by another user")
	}
	if len(got) != 0 {
		t.Fatalf("got %d entries, want 0 for a hidden timetable viewed by another user", len(got))
	}
}

func TestGetUserTimetable_BlockRelationReturnsEmpty(t *testing.T) {
	repo := &fakeListTimetableRepo{result: []*repository.TimetableEntryWithCourse{{}}}
	uc := NewGetUserTimetableUseCase(repo, nil, &fakeUserSettingRepo{value: "true", found: true}, &fakeBlockRepo{blocked: true})
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 1})

	y, s := 2026, "前期"
	got, visible, err := uc.Execute(ctx, 7, &y, &s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if visible {
		t.Fatal("visible = true, want false when the viewer and the owner have a block relation")
	}
	if len(got) != 0 {
		t.Fatalf("got %d entries, want 0 when the viewer and the owner have a block relation", len(got))
	}
}

// 閲覧可否の判定はユースケースの中だけにあるので、Execute の戻り値 visible で検証する
// （以前はリゾルバが同じ判定を呼び直していて、公開メソッドとして露出していた）。
func TestGetUserTimetable_VisibilityRules(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		found   bool
		blocked bool
		want    bool
	}{
		{name: "hidden", value: "false", found: true, want: false},
		{name: "shown", value: "true", found: true, want: true},
		{name: "unset defaults to visible", found: false, want: true},
		{name: "blocked overrides shown", value: "true", found: true, blocked: true, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeListTimetableRepo{result: []*repository.TimetableEntryWithCourse{{}}}
			uc := NewGetUserTimetableUseCase(repo, nil, &fakeUserSettingRepo{value: tc.value, found: tc.found}, &fakeBlockRepo{blocked: tc.blocked})
			ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 1})

			y, s := 2026, "前期"
			entries, visible, err := uc.Execute(ctx, 7, &y, &s)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if visible != tc.want {
				t.Fatalf("visible = %v, want %v", visible, tc.want)
			}
			if !tc.want && len(entries) != 0 {
				t.Fatalf("got %d entries, want 0 when the timetable is not visible", len(entries))
			}
		})
	}
}

func TestGetUserTimetable_OtherUserDefaultVisibleReturnsEntries(t *testing.T) {
	want := []*repository.TimetableEntryWithCourse{{}}
	repo := &fakeListTimetableRepo{result: want}
	uc := NewGetUserTimetableUseCase(repo, nil, &fakeUserSettingRepo{found: false}, &fakeBlockRepo{})
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 1})

	y, s := 2026, "前期"
	got, visible, err := uc.Execute(ctx, 7, &y, &s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !visible {
		t.Fatal("visible = false, want true (unset visibility defaults to visible)")
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d (unset visibility defaults to visible)", len(got), len(want))
	}
}
