package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListDedupKeysByYearUseCase interface {
	Execute(ctx context.Context, year int) (map[string]bool, error)
}

var _ ListDedupKeysByYearUseCase = &ListDedupKeysByYearInteractor{}

type ListDedupKeysByYearInteractor struct {
	courseRepo repository.CourseRepository
}

func NewListDedupKeysByYearUseCase(courseRepo repository.CourseRepository) ListDedupKeysByYearUseCase {
	return &ListDedupKeysByYearInteractor{courseRepo: courseRepo}
}

// Execute lists the dedup_key of every course already imported for year, so a
// re-scrape can skip expensive per-course work (scraper-side duplicate/campus
// disambiguation) for courses whose import is going to be a no-op anyway
// (auth/admin-role check is done by the resolver, matching ListCourseYearsUseCase).
func (uc *ListDedupKeysByYearInteractor) Execute(ctx context.Context, year int) (map[string]bool, error) {
	return uc.courseRepo.ListDedupKeysByYear(ctx, year)
}
