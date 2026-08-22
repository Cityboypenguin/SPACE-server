package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetCourseByIDUseCase interface {
	Execute(ctx context.Context, id int64) (*model.Course, error)
}

var _ GetCourseByIDUseCase = &GetCourseByIDInteractor{}

type GetCourseByIDInteractor struct {
	courseRepo repository.CourseRepository
}

func NewGetCourseByIDUseCase(courseRepo repository.CourseRepository) GetCourseByIDUseCase {
	return &GetCourseByIDInteractor{courseRepo: courseRepo}
}

func (uc *GetCourseByIDInteractor) Execute(ctx context.Context, id int64) (*model.Course, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, err
	}
	return uc.courseRepo.GetCourseByID(ctx, id)
}
