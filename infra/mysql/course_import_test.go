package mysql

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/repository"
	courseusecase "github.com/Cityboypenguin/SPACE-server/usecase/course"
)

func courseImportTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	db, cleanup := throwawaySchemaDB(t, "space_course_import_test", []string{
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
			day_of_week VARCHAR(16) NOT NULL,
			period INT NOT NULL,
			teacher_name VARCHAR(255) NOT NULL,
			course_name VARCHAR(255) NOT NULL,
			year INT NOT NULL,
			semester VARCHAR(32) NOT NULL,
			dedup_key VARCHAR(255) NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY unique_courses_dedup_key (dedup_key)
		)`,
	})
	return singleConnDB(db), cleanup
}

func scrapedCourses(n int, keyPrefix string) []courseusecase.ScrapedCourseInput {
	inputs := make([]courseusecase.ScrapedCourseInput, 0, n)
	for i := 0; i < n; i++ {
		inputs = append(inputs, courseusecase.ScrapedCourseInput{
			DayOfWeek:   "MON",
			Period:      i%5 + 1,
			TeacherName: "teacher",
			CourseName:  "course",
			Year:        2026,
			Semester:    "SPRING",
			DedupKey:    keyPrefix + "-" + strconv.Itoa(i),
		})
	}
	return inputs
}

// 取り込みは件数に依らず往復が一定であること。
//
// 以前は1件ごとに「FindByDedupKey して SaveCourseWithRoom」だったので、
// N 件で SELECT N 回 + INSERT 2N 回が並んでいた。シラバス1年ぶん（数千件）だと
// これがそのまま管理者の待ち時間になる。まとめた後は
// 存在確認 1 + rooms 1 + courses 1 に畳まれる。
func TestImportCourses_RoundTripsDoNotGrowWithInput(t *testing.T) {
	db, cleanup := courseImportTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewMySQLCourseRepository(db)
	uc := courseusecase.NewImportCoursesUseCase(repo)

	inputs := scrapedCourses(40, "k")

	var result *courseusecase.ImportCoursesResult
	var err error
	counts := countStatements(t, db, func() {
		result, err = uc.Execute(ctx, inputs)
	})
	if err != nil {
		t.Fatalf("importing the courses failed: %v", err)
	}

	if result.Imported != len(inputs) {
		t.Errorf("imported = %d, want %d", result.Imported, len(inputs))
	}
	if result.Skipped != 0 {
		t.Errorf("skipped = %d, want 0", result.Skipped)
	}
	if counts.inserts != 2 {
		t.Errorf("INSERT statements = %d, want 2 (rooms と courses で1本ずつ)", counts.inserts)
	}
	if counts.selects != 1 {
		t.Errorf("SELECT statements = %d, want 1 (存在確認は1本にまとめる)", counts.selects)
	}
}

// 保存した結果が1件ずつ保存していたときと同じであること。
// courses 1行につき rooms 1行が対応し、room_id の対応がずれていないことを確かめる
// （まとめた INSERT では採番を自分で振り直しているので、ここがずれると
// 「別の授業の部屋に入ってしまう」という形で壊れる）。
func TestImportCourses_LinksEachCourseToItsOwnRoom(t *testing.T) {
	db, cleanup := courseImportTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewMySQLCourseRepository(db)
	uc := courseusecase.NewImportCoursesUseCase(repo)

	inputs := []courseusecase.ScrapedCourseInput{
		{DayOfWeek: "MON", Period: 1, TeacherName: "t1", CourseName: "数学", Year: 2026, Semester: "SPRING", DedupKey: "k1"},
		{DayOfWeek: "TUE", Period: 2, TeacherName: "t2", CourseName: "物理", Year: 2026, Semester: "SPRING", DedupKey: "k2"},
		{DayOfWeek: "WED", Period: 3, TeacherName: "t3", CourseName: "化学", Year: 2026, Semester: "AUTUMN", DedupKey: "k3"},
	}
	if _, err := uc.Execute(ctx, inputs); err != nil {
		t.Fatalf("importing the courses failed: %v", err)
	}

	for _, in := range inputs {
		c, err := repo.FindByDedupKey(ctx, in.DedupKey)
		if err != nil {
			t.Fatalf("looking up %s failed: %v", in.DedupKey, err)
		}
		if c == nil {
			t.Fatalf("%s was not stored", in.DedupKey)
		}
		if c.CourseName != in.CourseName || c.DayOfWeek != in.DayOfWeek || c.Period != in.Period ||
			c.TeacherName != in.TeacherName || c.Year != in.Year || c.Semester != in.Semester {
			t.Errorf("%s stored as %+v, does not match the input %+v", in.DedupKey, c, in)
		}

		// その授業の room がその授業の名前で作られていること。
		var roomName, roomType string
		if err := db.QueryRow(`SELECT name, type FROM rooms WHERE id = ?`, c.RoomID).Scan(&roomName, &roomType); err != nil {
			t.Fatalf("looking up the room for %s failed: %v", in.DedupKey, err)
		}
		if roomName != in.CourseName {
			t.Errorf("%s is linked to the room %q, want %q (room_id の対応がずれている)", in.DedupKey, roomName, in.CourseName)
		}
		if roomType != "course" {
			t.Errorf("%s is linked to a room of type %q, want \"course\"", in.DedupKey, roomType)
		}
	}
}

// 再実行は何も作らず、全件 skip になること（DedupKey による冪等性）。
// 入力の中に同じ DedupKey が重複していても unique index 違反で丸ごと失敗しないこと。
func TestImportCourses_IsIdempotentAndDropsDuplicateKeys(t *testing.T) {
	db, cleanup := courseImportTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewMySQLCourseRepository(db)
	uc := courseusecase.NewImportCoursesUseCase(repo)

	base := []courseusecase.ScrapedCourseInput{
		{DayOfWeek: "MON", Period: 1, TeacherName: "t1", CourseName: "数学", Year: 2026, Semester: "SPRING", DedupKey: "k1"},
		{DayOfWeek: "TUE", Period: 2, TeacherName: "t2", CourseName: "物理", Year: 2026, Semester: "SPRING", DedupKey: "k2"},
	}

	first, err := uc.Execute(ctx, base)
	if err != nil {
		t.Fatalf("the first import failed: %v", err)
	}
	if first.Imported != 2 || first.Skipped != 0 {
		t.Fatalf("first import = %+v, want imported 2 / skipped 0", first)
	}

	// 同じ入力をもう一度。既に在るので全件 skip。
	second, err := uc.Execute(ctx, base)
	if err != nil {
		t.Fatalf("the second import failed: %v", err)
	}
	if second.Imported != 0 || second.Skipped != 2 {
		t.Errorf("second import = %+v, want imported 0 / skipped 2", second)
	}

	// 入力そのものに重複がある場合。新規1件だけが入り、重複したぶんは skip。
	withDupes := []courseusecase.ScrapedCourseInput{
		{DayOfWeek: "WED", Period: 3, TeacherName: "t3", CourseName: "化学", Year: 2026, Semester: "AUTUMN", DedupKey: "k3"},
		{DayOfWeek: "WED", Period: 3, TeacherName: "t3", CourseName: "化学", Year: 2026, Semester: "AUTUMN", DedupKey: "k3"},
		{DayOfWeek: "MON", Period: 1, TeacherName: "t1", CourseName: "数学", Year: 2026, Semester: "SPRING", DedupKey: "k1"},
	}
	third, err := uc.Execute(ctx, withDupes)
	if err != nil {
		t.Fatalf("importing duplicated rows failed: %v", err)
	}
	if third.Imported != 1 || third.Skipped != 2 {
		t.Errorf("import with duplicates = %+v, want imported 1 / skipped 2", third)
	}

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM courses`).Scan(&total); err != nil {
		t.Fatalf("counting the courses failed: %v", err)
	}
	if total != 3 {
		t.Errorf("courses = %d, want 3", total)
	}

	// rooms も同数（部屋だけ余分に出来ていないこと）。
	var rooms int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rooms`).Scan(&rooms); err != nil {
		t.Fatalf("counting the rooms failed: %v", err)
	}
	if rooms != 3 {
		t.Errorf("rooms = %d, want 3 (授業1件につき部屋1件)", rooms)
	}
}

// FindExistingDedupKeys は在るものだけを true にすること。
func TestFindExistingDedupKeys_ReportsOnlyStoredKeys(t *testing.T) {
	db, cleanup := courseImportTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewMySQLCourseRepository(db)

	if _, err := repo.SaveCoursesWithRooms(ctx, []repository.SaveCourseParam{
		{DayOfWeek: "MON", Period: 1, TeacherName: "t", CourseName: "c1", Year: 2026, Semester: "SPRING", DedupKey: "here1"},
		{DayOfWeek: "TUE", Period: 2, TeacherName: "t", CourseName: "c2", Year: 2026, Semester: "SPRING", DedupKey: "here2"},
	}); err != nil {
		t.Fatalf("seeding failed: %v", err)
	}

	got, err := repo.FindExistingDedupKeys(ctx, []string{"here1", "missing", "here2"})
	if err != nil {
		t.Fatalf("FindExistingDedupKeys failed: %v", err)
	}
	if !got["here1"] || !got["here2"] {
		t.Errorf("stored keys were not reported: %v", got)
	}
	if got["missing"] {
		t.Error("an absent key was reported as existing")
	}

	// 空入力では DB へ行かずに空を返すこと（IN () は構文エラー）。
	empty, err := repo.FindExistingDedupKeys(ctx, nil)
	if err != nil {
		t.Fatalf("FindExistingDedupKeys with no keys failed: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("empty input returned %v, want an empty map", empty)
	}
}
