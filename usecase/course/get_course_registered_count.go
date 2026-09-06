package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// GetCourseRegisteredCountUseCase reports how many users currently have a course
// in their timetable, so an admin can see the blast radius before deleting it (the
// delete cascades away every one of those registrations, plus the course room's
// entire message/question/poll history).
type GetCourseRegisteredCountUseCase interface {
	Execute(ctx context.Context, courseID int64) (int, error)
}

var _ GetCourseRegisteredCountUseCase = &GetCourseRegisteredCountInteractor{}

type GetCourseRegisteredCountInteractor struct {
	timetableRepo repository.TimetableRepository
}

func NewGetCourseRegisteredCountUseCase(timetableRepo repository.TimetableRepository) GetCourseRegisteredCountUseCase {
	return &GetCourseRegisteredCountInteractor{timetableRepo: timetableRepo}
}

// Execute is resolver-gated to admins (see Course.registeredCount in
// schema.resolvers.go), matching this package's existing convention of leaving
// auth/admin-role checks to the resolver rather than the use case.
func (uc *GetCourseRegisteredCountInteractor) Execute(ctx context.Context, courseID int64) (int, error) {
	return uc.timetableRepo.CountByCourseID(ctx, courseID)
}
