// Command scraper is a one-off/periodic batch job: it pulls the full course
// catalog for a given academic year from the public 専修大学 syllabus site and
// syncs the courses table to it (new courses are created together with their chat
// room, changed ones are updated, ones that vanished are marked discontinued). The
// same sync runs from the admin screen (adminTriggerCourseImport); this command is
// for an operator with direct DB access, e.g.:
//
//	go run ./cmd/scraper -year 2026 -admin-id 1 -dry-run
//	go run ./cmd/scraper -year 2026 -admin-id 1
//
// -admin-id is the administrator the run is recorded as (course_sync_runs.triggered_by);
// the sync use case only runs for an administrator, same as from the admin screen.
// Start with -dry-run: it records what would change without touching any course.
package main

import (
	"context"
	"flag"
	"time"

	"github.com/Cityboypenguin/SPACE-server/infra/mysql"
	"github.com/Cityboypenguin/SPACE-server/infra/scraper"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	courseusecase "github.com/Cityboypenguin/SPACE-server/usecase/course"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

// offlineDelivery はリアルタイム配信をしない UserEventDelivery。
type offlineDelivery struct{}

func (offlineDelivery) PublishToUser(int64, string, map[string]any) {}
func (offlineDelivery) ConnectedUserIDs() []int64                   { return nil }

func main() {
	year := flag.Int("year", time.Now().Year(), "academic year to import (e.g. 2026)")
	adminID := flag.Int64("admin-id", 0, "administrator ID to record the run as (required)")
	dryRun := flag.Bool("dry-run", false, "only record what would change; do not modify any course")
	flag.Parse()

	if *adminID <= 0 {
		logger.Log.Fatal().Msg("-admin-id is required")
	}

	database, err := mysql.New()
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to connect to database")
	}

	courseRepository := mysql.NewMySQLCourseRepository(database)
	// このコマンドには SSE の接続が無いので、時間割から外した人への通知は DB に残すだけ
	// （本人が次にアプリを開いたときに通知一覧に出る）。
	notifier := notificationuc.NewNotificationPublisher(mysql.NewMySQLNotificationRepository(database), offlineDelivery{})
	syncCourses := courseusecase.NewSyncCoursesUseCase(courseRepository, mysql.NewMySQLCourseSyncRepository(database), mysql.NewMySQLTxManager(database), notifier)

	syllabusScraper, err := scraper.NewSenshuSyllabusScraper()
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to initialize scraper")
	}
	defer syllabusScraper.Close()

	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: *adminID, Role: "admin"})
	startedAt := time.Now()

	knownDedupKeys, err := courseRepository.ListDedupKeysByYear(ctx, *year)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to list already-imported courses")
	}

	logger.Log.Info().Int("year", *year).Int("already_imported", len(knownDedupKeys)).Bool("dry_run", *dryRun).Msg("fetching course catalog")
	lastPercent := -1
	fetched, err := syllabusScraper.FetchCourses(ctx, *year, knownDedupKeys, func(done, total int) {
		percent := 0
		if total > 0 {
			percent = done * 100 / total
		}
		if percent == lastPercent {
			return
		}
		lastPercent = percent
		logger.Log.Info().Int("fetched", done).Int("total", total).Int("percent", percent).Msg("scraping progress")
	})
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to fetch course catalog")
	}
	logger.Log.Info().
		Int("fetched", len(fetched.Courses)).
		Int("skipped_unmappable_slot", fetched.SkippedSlots).
		Int("site_total", fetched.Completeness.SiteTotal).
		Int("listed_rows", fetched.Completeness.ListedRows).
		Msg("fetch complete; syncing")

	run, err := syncCourses.Execute(ctx, courseusecase.SyncCoursesInput{
		Year:         *year,
		DryRun:       *dryRun,
		Courses:      fetched.Courses,
		Completeness: fetched.Completeness,
		StartedAt:    startedAt,
	})
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to sync courses")
	}

	logger.Log.Info().
		Int64("run_id", run.ID).
		Bool("dry_run", run.DryRun).
		Int("created", run.Created).
		Int("updated", run.Updated).
		Int("discontinued", run.Discontinued).
		Int("restored", run.Restored).
		Int("reviews", run.Reviews).
		Int("unchanged", run.Unchanged).
		Int("unregistered", run.Unregistered).
		Str("discontinue_skipped_reason", run.DiscontinueSkippedReason).
		Msg("sync complete")
}
