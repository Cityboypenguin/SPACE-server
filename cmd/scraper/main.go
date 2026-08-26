// Command scraper is a one-off/periodic batch job: it pulls the full course
// catalog for a given academic year from the public 専修大学 syllabus site and
// upserts it into the courses table (each new course also gets its chat room via
// SaveCourseWithRoom). It is intentionally not wired into cmd/server or exposed as
// a GraphQL mutation yet — it is meant to be run manually by an operator with
// direct DB access, e.g.:
//
//	go run ./cmd/scraper -year 2026
package main

import (
	"context"
	"flag"
	"time"

	"github.com/Cityboypenguin/SPACE-server/infra/mysql"
	"github.com/Cityboypenguin/SPACE-server/infra/scraper"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	courseusecase "github.com/Cityboypenguin/SPACE-server/usecase/course"
)

func main() {
	year := flag.Int("year", time.Now().Year(), "academic year to import (e.g. 2026)")
	flag.Parse()

	database, err := mysql.New()
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to connect to database")
	}

	courseRepository := mysql.NewMySQLCourseRepository(database)
	importCourses := courseusecase.NewImportCoursesUseCase(courseRepository)

	syllabusScraper, err := scraper.NewSenshuSyllabusScraper()
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to initialize scraper")
	}
	defer syllabusScraper.Close()

	ctx := context.Background()

	knownDedupKeys, err := courseRepository.ListDedupKeysByYear(ctx, *year)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to list already-imported courses")
	}

	logger.Log.Info().Int("year", *year).Int("already_imported", len(knownDedupKeys)).Msg("fetching course catalog")
	scraped, skipped, err := syllabusScraper.FetchCourses(ctx, *year, knownDedupKeys)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to fetch course catalog")
	}
	logger.Log.Info().
		Int("fetched", len(scraped)).
		Int("skipped_unmappable_slot", skipped).
		Msg("fetch complete; importing")

	result, err := importCourses.Execute(ctx, scraped)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to import courses")
	}

	logger.Log.Info().
		Int("imported", result.Imported).
		Int("already_present", result.Skipped).
		Msg("import complete")
}
