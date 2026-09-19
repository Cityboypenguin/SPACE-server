package course

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type deleteCourseRepo struct {
	repository.CourseRepository
	called bool
}

func (r *deleteCourseRepo) GetCourseByID(context.Context, int64) (*model.Course, error) {
	r.called = true
	return &model.Course{RoomID: 8}, nil
}

type deleteCourseRoomRepo struct {
	repository.RoomRepository
	called bool
}

func (r *deleteCourseRoomRepo) DeleteRoom(context.Context, int64) (bool, error) {
	r.called = true
	return true, nil
}

func TestAdminDeleteCourseRequiresAdminInUseCase(t *testing.T) {
	courses := &deleteCourseRepo{}
	rooms := &deleteCourseRoomRepo{}
	uc := NewAdminDeleteCourseUseCase(courses, rooms)
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "user"})
	if _, err := uc.Execute(ctx, 7); err == nil {
		t.Fatal("non-admin must be rejected")
	}
	if courses.called || rooms.called {
		t.Fatal("repositories must not be called before authorization")
	}
}
