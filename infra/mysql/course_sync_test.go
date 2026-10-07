package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	courseusecase "github.com/Cityboypenguin/SPACE-server/usecase/course"
	"github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

// ■ シラバス同期を実物の MySQL に通すテスト
//
// 照合の規則そのものは usecase/course/sync_plan_test.go で見ている。ここで確かめたいのは
// 「計画どおりに書いたとき、時間割・ルーム・記録がどうなるか」で、SQL の意味に依る。

func courseSyncTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	db, cleanup := throwawaySchemaDB(t, "space_course_sync_test", []string{
		`CREATE TABLE rooms (
			id BIGINT NOT NULL AUTO_INCREMENT,
			name VARCHAR(255) NOT NULL,
			type VARCHAR(32) NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE courses (
			id BIGINT NOT NULL AUTO_INCREMENT,
			room_id BIGINT NOT NULL,
			day_of_week VARCHAR(10) NOT NULL,
			period INT NOT NULL,
			teacher_name VARCHAR(255) NOT NULL,
			course_name VARCHAR(255) NOT NULL,
			year INT NOT NULL,
			semester VARCHAR(10) NOT NULL,
			dedup_key VARCHAR(255) NOT NULL,
			source VARCHAR(20) NOT NULL DEFAULT 'senshu',
			source_ref VARCHAR(64) NULL,
			source_name VARCHAR(255) NULL,
			discontinued_at BIGINT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY unique_courses_dedup_key (dedup_key)
		)`,
		`CREATE TABLE timetables (
			id BIGINT NOT NULL AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			course_id BIGINT NOT NULL,
			color VARCHAR(20) NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY unique_timetables_user_course (user_id, course_id)
		)`,
		`CREATE TABLE course_sync_reviews (
			id BIGINT NOT NULL AUTO_INCREMENT,
			year INT NOT NULL,
			kind VARCHAR(32) NOT NULL,
			fingerprint CHAR(64) NOT NULL,
			status VARCHAR(16) NOT NULL,
			message VARCHAR(1000) NOT NULL,
			existing_json JSON NOT NULL,
			proposed_json JSON NOT NULL,
			first_run_id BIGINT NULL,
			applied_run_id BIGINT NULL,
			resolved_by BIGINT NULL,
			resolved_at BIGINT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY unique_course_sync_reviews_fingerprint (fingerprint)
		)`,
		`CREATE TABLE course_sync_runs (
			id BIGINT NOT NULL AUTO_INCREMENT,
			year INT NOT NULL,
			dry_run BOOLEAN NOT NULL,
			triggered_by BIGINT NULL,
			site_total INT NOT NULL,
			listed_rows INT NOT NULL,
			unidentified_rows INT NOT NULL,
			created_count INT NOT NULL,
			updated_count INT NOT NULL,
			discontinued_count INT NOT NULL,
			restored_count INT NOT NULL,
			review_count INT NOT NULL,
			unchanged_count INT NOT NULL,
			unregistered_count INT NOT NULL,
			discontinue_skipped_reason VARCHAR(255) NULL,
			started_at BIGINT NOT NULL,
			finished_at BIGINT NOT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE course_sync_changes (
			id BIGINT NOT NULL AUTO_INCREMENT,
			run_id BIGINT NOT NULL,
			kind VARCHAR(16) NOT NULL,
			course_id BIGINT NULL,
			review_id BIGINT NULL,
			course_name VARCHAR(255) NOT NULL,
			teacher_name VARCHAR(255) NOT NULL,
			detail VARCHAR(1000) NOT NULL,
			before_json JSON NULL,
			after_json JSON NULL,
			registered_count INT NOT NULL,
			unregistered_count INT NOT NULL,
			PRIMARY KEY (id)
		)`,
	})
	return singleConnDB(db), cleanup
}

// recordingNotifier は通知を DB にも SSE にも出さず、送ろうとした内容だけを残す。
type recordingNotifier struct {
	notification.NotificationPublisher
	sent []notification.PublishParams
}

func (n *recordingNotifier) PublishBatch(_ context.Context, params []notification.PublishParams) error {
	n.sent = append(n.sent, params...)
	return nil
}

type syncFixture struct {
	t         *testing.T
	db        *sql.DB
	ctx       context.Context
	courses   repository.CourseRepository
	syncRepo  repository.CourseSyncRepository
	timetable repository.TimetableRepository
	sync      courseusecase.SyncCoursesUseCase
	admin     courseusecase.CourseSyncAdminUseCase
	notifier  *recordingNotifier
}

func newSyncFixture(t *testing.T) (*syncFixture, func()) {
	db, cleanup := courseSyncTestDB(t)
	courses := NewMySQLCourseRepository(db)
	syncRepo := NewMySQLCourseSyncRepository(db)
	notifier := &recordingNotifier{}
	return &syncFixture{
		t:         t,
		db:        db,
		ctx:       auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "admin"}),
		courses:   courses,
		syncRepo:  syncRepo,
		timetable: NewMySQLTimetableRepository(db),
		sync:      courseusecase.NewSyncCoursesUseCase(courses, syncRepo, NewMySQLTxManager(db), notifier),
		admin:     courseusecase.NewCourseSyncAdminUseCase(syncRepo),
		notifier:  notifier,
	}, cleanup
}

func syllabusRow(ref, day string, period int, name, teacher string) courseusecase.ScrapedCourseInput {
	return courseusecase.ScrapedCourseInput{
		Year: 2026, Semester: model.SemesterFirst, DayOfWeek: day, Period: period,
		CourseName: name, SourceName: name, TeacherName: teacher, SourceRef: ref,
		DedupKey: fmt.Sprintf("senshu:2026:前期:%s:%s:%d", ref, day, period),
	}
}

// baseSyllabus は経済学入門（水5）、日本文化論（火2）、統計学（水4）と、変化させない18件。
func baseSyllabus() []courseusecase.ScrapedCourseInput {
	rows := []courseusecase.ScrapedCourseInput{
		syllabusRow("A", "水", 5, "経済学入門", "田中"),
		syllabusRow("B", "火", 2, "日本文化論", "鈴木"),
		syllabusRow("X", "水", 4, "統計学", "山田"),
	}
	for i := 0; i < 18; i++ {
		ref := fmt.Sprintf("F%02d", i)
		rows = append(rows, syllabusRow(ref, "土", 1, "固定"+ref, "固定先生"))
	}
	return rows
}

func (f *syncFixture) run(rows []courseusecase.ScrapedCourseInput, dryRun bool) *model.CourseSyncRun {
	f.t.Helper()
	run, err := f.sync.Execute(f.ctx, courseusecase.SyncCoursesInput{
		Year:         2026,
		DryRun:       dryRun,
		Courses:      rows,
		Completeness: courseusecase.FetchCompleteness{SiteTotal: len(rows), ListedRows: len(rows)},
		StartedAt:    time.Now(),
	})
	if err != nil {
		f.t.Fatalf("sync failed: %v", err)
	}
	return run
}

func (f *syncFixture) course(ref string) *model.Course {
	f.t.Helper()
	var id int64
	if err := f.db.QueryRow(`SELECT id FROM courses WHERE source_ref = ? ORDER BY id LIMIT 1`, ref).Scan(&id); err != nil {
		f.t.Fatalf("looking up course %s: %v", ref, err)
	}
	c, err := f.courses.GetCourseByID(f.ctx, id)
	if err != nil || c == nil {
		f.t.Fatalf("reading course %d: %v", id, err)
	}
	return c
}

func (f *syncFixture) changes(runID int64) map[int64]*model.CourseSyncChange {
	f.t.Helper()
	list, _, err := f.syncRepo.ListChanges(f.ctx, runID, nil, repository.PageQuery{Limit: 100})
	if err != nil {
		f.t.Fatalf("listing changes: %v", err)
	}
	out := make(map[int64]*model.CourseSyncChange)
	for _, c := range list {
		if c.CourseID != nil {
			out[*c.CourseID] = c
		}
	}
	return out
}

func (f *syncFixture) count(query string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.db.QueryRow(query, args...).Scan(&n); err != nil {
		f.t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func TestCourseSync_EndToEnd(t *testing.T) {
	f, cleanup := newSyncFixture(t)
	defer cleanup()

	// 1回目: 全部が新規。授業1件につきルーム1件、ルーム名は授業名。
	first := f.run(baseSyllabus(), false)
	if first.Created != 21 || first.ID == 0 {
		t.Fatalf("first run = %+v, want 21 created", first)
	}
	if n := f.count(`SELECT COUNT(*) FROM rooms`); n != 21 {
		t.Fatalf("rooms = %d, want 21", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM courses c JOIN rooms r ON r.id = c.room_id WHERE r.name = c.course_name`); n != 21 {
		t.Fatalf("courses linked to a room of their own name = %d, want 21", n)
	}

	a, b, x := f.course("A"), f.course("B"), f.course("X")
	// 利用者1は経済学入門（水5）と統計学（水4）、利用者2は日本文化論、
	// 利用者3は経済学入門だけを登録している。
	for _, reg := range []struct{ user, course int64 }{{1, a.ID}, {1, x.ID}, {2, b.ID}, {3, a.ID}} {
		if _, err := f.timetable.Upsert(f.ctx, reg.user, reg.course); err != nil {
			t.Fatalf("registering: %v", err)
		}
	}

	// 2回目: 経済学入門が水5→水4、日本文化論が消える、統計学の教員が変わる。
	rows := baseSyllabus()
	rows[0] = syllabusRow("A", "水", 4, "経済学入門", "田中")
	rows[2] = syllabusRow("X", "水", 4, "統計学", "佐藤")
	rows = append(rows[:1], rows[2:]...)

	// 先にドライラン。外れる人数まで見積もるが、授業にも時間割にも触らない。
	preview := f.run(rows, true)
	if !preview.DryRun || preview.Updated != 2 || preview.Discontinued != 1 || preview.Unregistered != 1 {
		t.Fatalf("dry run = %+v, want 2 updated / 1 discontinued / 1 unregistered", preview)
	}
	if c := f.course("A"); c.Period != 5 {
		t.Fatalf("the dry run moved course A to %s%d", c.DayOfWeek, c.Period)
	}
	if n := f.count(`SELECT COUNT(*) FROM timetables`); n != 4 {
		t.Fatalf("timetables after the dry run = %d, want all 4 kept", n)
	}
	if f.course("B").DiscontinuedAt != nil {
		t.Fatal("the dry run discontinued course B")
	}
	if len(f.notifier.sent) != 0 {
		t.Fatalf("the dry run sent %d notification(s)", len(f.notifier.sent))
	}
	if change := f.changes(preview.ID)[a.ID]; change == nil || change.UnregisteredCount != 1 {
		t.Fatalf("dry run change for A = %+v, want 1 to be unregistered", change)
	}

	second := f.run(rows, false)
	if second.Created != 0 || second.Updated != 2 || second.Discontinued != 1 || second.Unchanged != 18 {
		t.Fatalf("second run = %+v, want 0 created / 2 updated / 1 discontinued / 18 unchanged", second)
	}

	t.Run("a slot move keeps the course and its room, and one course per slot", func(t *testing.T) {
		moved := f.course("A")
		if moved.ID != a.ID || moved.RoomID != a.RoomID {
			t.Fatalf("course A is now id %d / room %d, want the same %d / %d", moved.ID, moved.RoomID, a.ID, a.RoomID)
		}
		if moved.DayOfWeek != "水" || moved.Period != 4 {
			t.Fatalf("course A is at %s%d, want 水4", moved.DayOfWeek, moved.Period)
		}
		// 利用者1の水4には統計学があるので、移ってきた経済学入門の方を外す。
		// 重ならない利用者3の登録は残る。
		if n := f.count(`SELECT COUNT(*) FROM timetables WHERE user_id = 1 AND course_id = ?`, a.ID); n != 0 {
			t.Errorf("user 1 still has course A; want it removed so 水4 holds one course")
		}
		if n := f.count(`SELECT COUNT(*) FROM timetables WHERE user_id = 1 AND course_id = ?`, x.ID); n != 1 {
			t.Errorf("user 1 lost course X; want the course already in 水4 kept")
		}
		if n := f.count(`SELECT COUNT(*) FROM timetables WHERE user_id = 3 AND course_id = ?`, a.ID); n != 1 {
			t.Errorf("user 3 lost course A although nothing else is in 水4")
		}
		if second.Unregistered != 1 {
			t.Errorf("run unregistered = %d, want 1", second.Unregistered)
		}
		change := f.changes(second.ID)[a.ID]
		if change == nil || change.Kind != model.CourseSyncChangeUpdated {
			t.Fatalf("change for A = %+v, want UPDATED", change)
		}
		if change.RegisteredCount != 2 || change.UnregisteredCount != 1 {
			t.Errorf("change for A: registered %d / unregistered %d, want 2 / 1", change.RegisteredCount, change.UnregisteredCount)
		}
		if len(f.notifier.sent) != 1 {
			t.Fatalf("notifications = %d, want 1", len(f.notifier.sent))
		}
		sent := f.notifier.sent[0]
		if sent.UserID != 1 || sent.Type != notification.TypeTimetableRemoved || sent.TargetID == nil || *sent.TargetID != a.ID {
			t.Errorf("notification = %+v, want user 1 told about course A", sent)
		}
		if !strings.Contains(sent.Message, "経済学入門") || !strings.Contains(sent.Message, "統計学") || !strings.Contains(sent.Message, "水曜4限") {
			t.Errorf("notification message = %q", sent.Message)
		}
		if change.Before == nil || change.Before.Period != 5 || change.After == nil || change.After.Period != 4 {
			t.Errorf("change for A: before %+v / after %+v", change.Before, change.After)
		}
	})

	t.Run("a vanished course is marked, not deleted", func(t *testing.T) {
		gone := f.course("B")
		if gone.DiscontinuedAt == nil {
			t.Fatal("course B is not marked discontinued")
		}
		entries, err := f.timetable.ListByUser(f.ctx, 2, 2026, model.SemesterFirst)
		if err != nil {
			t.Fatalf("listing user 2's timetable: %v", err)
		}
		if len(entries) != 1 || entries[0].Course.ID != b.ID || !entries[0].Course.IsDiscontinued() {
			t.Fatalf("user 2's timetable = %+v, want course B kept and shown as discontinued", entries)
		}
		found, _, err := f.courses.SearchByDayPeriod(f.ctx, "火", 2, "", 2026, model.SemesterFirst, repository.PageQuery{Limit: 10})
		if err != nil {
			t.Fatalf("searching: %v", err)
		}
		if len(found) != 0 {
			t.Errorf("search still returns %d course(s) for 火2, want the discontinued one hidden", len(found))
		}
		if _, err := f.timetable.Upsert(f.ctx, 3, b.ID); !errors.Is(err, repository.ErrCourseDiscontinued) {
			t.Errorf("registering a discontinued course: err = %v, want ErrCourseDiscontinued", err)
		}
		change := f.changes(second.ID)[b.ID]
		if change == nil || change.Kind != model.CourseSyncChangeDiscontinued || change.RegisteredCount != 1 {
			t.Errorf("change for B = %+v, want DISCONTINUED with 1 registrant", change)
		}
	})

	t.Run("a dry run records the changes without touching courses", func(t *testing.T) {
		renamed := append([]courseusecase.ScrapedCourseInput{}, rows...)
		renamed[1] = syllabusRow("X", "水", 4, "統計学Ⅰ", "佐藤")
		before := f.course("X")
		dry := f.run(renamed, true)
		if !dry.DryRun || dry.Updated != 1 {
			t.Fatalf("dry run = %+v, want 1 update recorded", dry)
		}
		if after := f.course("X"); after.CourseName != before.CourseName {
			t.Fatalf("dry run renamed course X to %q", after.CourseName)
		}
		if change := f.changes(dry.ID)[x.ID]; change == nil || change.After.CourseName != "統計学Ⅰ" {
			t.Errorf("dry run change for X = %+v, want the planned rename", change)
		}

		real := f.run(renamed, false)
		if real.Updated != 1 {
			t.Fatalf("real run = %+v, want 1 update", real)
		}
		var roomName string
		if err := f.db.QueryRow(`SELECT name FROM rooms WHERE id = ?`, x.RoomID).Scan(&roomName); err != nil {
			t.Fatalf("reading room: %v", err)
		}
		if roomName != "統計学Ⅰ" {
			t.Errorf("room name = %q, want it renamed with the course", roomName)
		}
		rows = renamed
	})

	t.Run("a re-run with nothing new changes nothing", func(t *testing.T) {
		again := f.run(rows, false)
		if again.Created+again.Updated+again.Discontinued+again.Restored+again.Reviews != 0 {
			t.Fatalf("re-run = %+v, want no changes", again)
		}
	})

	t.Run("a suspected reused code waits for an admin and is applied on the next sync", func(t *testing.T) {
		reused := append([]courseusecase.ScrapedCourseInput{}, rows...)
		reused[2] = syllabusRow("F00", "土", 1, "まったく別の授業", "別の先生")
		before := f.course("F00")

		held := f.run(reused, false)
		if held.Reviews != 1 || held.Updated != 0 {
			t.Fatalf("run = %+v, want 1 review and no update", held)
		}
		if c := f.course("F00"); c.CourseName != before.CourseName {
			t.Fatalf("course F00 was renamed to %q while under review", c.CourseName)
		}
		// 再実行しても確認は積み増さない。
		f.run(reused, false)
		pending := model.CourseSyncReviewPending
		reviews, _, err := f.admin.ListReviews(f.ctx, repository.ListCourseSyncReviewsParam{Status: &pending, Page: repository.PageQuery{Limit: 10}})
		if err != nil {
			t.Fatalf("listing reviews: %v", err)
		}
		if len(reviews) != 1 || reviews[0].Kind != model.CourseSyncReviewCodeReused {
			t.Fatalf("pending reviews = %+v, want exactly one CODE_REUSED", reviews)
		}

		if _, err := f.admin.ResolveReview(f.ctx, reviews[0].ID, model.CourseSyncReviewSame); err != nil {
			t.Fatalf("resolving: %v", err)
		}
		if _, err := f.admin.ResolveReview(f.ctx, reviews[0].ID, model.CourseSyncReviewDifferent); err == nil {
			t.Error("resolving an already-resolved review succeeded, want a conflict")
		}

		applied := f.run(reused, false)
		if applied.Updated != 1 || applied.Reviews != 0 {
			t.Fatalf("run after the decision = %+v, want 1 update and no review", applied)
		}
		c := f.course("F00")
		if c.ID != before.ID || c.CourseName != "まったく別の授業" || c.TeacherName != "別の先生" {
			t.Errorf("course F00 = %+v, want it updated in place", c)
		}
		rv, err := f.syncRepo.GetReview(f.ctx, reviews[0].ID)
		if err != nil || rv.Status != model.CourseSyncReviewApplied || rv.AppliedRunID == nil || *rv.AppliedRunID != applied.ID {
			t.Errorf("review after the sync = %+v (err %v), want APPLIED by run %d", rv, err, applied.ID)
		}
	})
}

// 「別の授業」と判断した使い回しは、旧授業を退避してから同じ dedup_key で
// 新しい授業を作る（unique index に当たらないこと）。旧授業の時間割は残る。
func TestCourseSync_RetireThenRecreateWithTheSameKey(t *testing.T) {
	f, cleanup := newSyncFixture(t)
	defer cleanup()

	f.run(baseSyllabus(), false)
	old := f.course("A")
	if _, err := f.timetable.Upsert(f.ctx, 1, old.ID); err != nil {
		t.Fatalf("registering: %v", err)
	}

	reused := baseSyllabus()
	reused[0] = syllabusRow("A", "水", 5, "まったく別の授業", "別の先生")
	f.run(reused, false)
	reviews, err := f.syncRepo.ListReviewsByYear(f.ctx, 2026)
	if err != nil || len(reviews) != 1 {
		t.Fatalf("reviews = %v (err %v), want 1", reviews, err)
	}
	if _, err := f.admin.ResolveReview(f.ctx, reviews[0].ID, model.CourseSyncReviewDifferent); err != nil {
		t.Fatalf("resolving: %v", err)
	}

	run := f.run(reused, false)
	if run.Created != 1 || run.Discontinued != 1 {
		t.Fatalf("run = %+v, want 1 created / 1 discontinued", run)
	}
	retired, err := f.courses.GetCourseByID(f.ctx, old.ID)
	if err != nil || retired == nil || retired.DiscontinuedAt == nil || retired.SourceRef != "" {
		t.Fatalf("old course = %+v (err %v), want it discontinued and out of matching", retired, err)
	}
	fresh := f.course("A")
	if fresh.ID == old.ID || fresh.CourseName != "まったく別の授業" || fresh.RoomID == old.RoomID {
		t.Fatalf("new course = %+v, want a separate course with its own room", fresh)
	}
	if n := f.count(`SELECT COUNT(*) FROM timetables WHERE course_id = ?`, old.ID); n != 1 {
		t.Errorf("registrations of the old course = %d, want 1 kept", n)
	}
}

// 2つの授業がどちらも同じコマへ移ってきたら、先に登録していた方を残す。
func TestCourseSync_TwoCoursesMovingIntoOneSlotKeepTheEarlierRegistration(t *testing.T) {
	f, cleanup := newSyncFixture(t)
	defer cleanup()

	f.run(baseSyllabus(), false)
	a, b := f.course("A"), f.course("B")
	for _, id := range []int64{b.ID, a.ID} { // B を先に登録
		if _, err := f.timetable.Upsert(f.ctx, 1, id); err != nil {
			t.Fatalf("registering: %v", err)
		}
	}

	rows := baseSyllabus()
	rows[0] = syllabusRow("A", "金", 1, "経済学入門", "田中")
	rows[1] = syllabusRow("B", "金", 1, "日本文化論", "鈴木")
	run := f.run(rows, false)
	if run.Unregistered != 1 {
		t.Fatalf("unregistered = %d, want 1", run.Unregistered)
	}
	if n := f.count(`SELECT COUNT(*) FROM timetables WHERE user_id = 1 AND course_id = ?`, b.ID); n != 1 {
		t.Error("the earlier registration (B) was removed")
	}
	if n := f.count(`SELECT COUNT(*) FROM timetables WHERE user_id = 1 AND course_id = ?`, a.ID); n != 0 {
		t.Error("the later registration (A) was kept; want one course per slot")
	}
}
