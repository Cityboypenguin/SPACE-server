package mysql

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// The integration must retain sync metadata while adding dev_new's counts and sorting.
func TestCourseSearchAndAdminCountsWithSyncColumns(t *testing.T) {
	db, cleanup := courseSyncTestDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewMySQLCourseRepository(db)
	first, err := repo.SaveCourseWithRoom(ctx, repository.SaveCourseParam{DayOfWeek: "月", Period: 1, TeacherName: "先生", CourseName: "授業A", Year: 2026, Semester: "前期", DedupKey: "a", Source: model.CourseSourceSenshu, SourceRef: "ref-a", SourceName: "元の授業名"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.SaveCourseWithRoom(ctx, repository.SaveCourseParam{DayOfWeek: "月", Period: 1, TeacherName: "先生", CourseName: "授業B", Year: 2026, Semester: "前期", DedupKey: "b", Source: model.CourseSourceManual})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO messages VALUES(1,?), (2,?)`, first.RoomID, first.RoomID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO timetables(user_id,course_id,color,created_at,updated_at) VALUES(1,?,'red',1,1),(2,?,'blue',1,1)`, first.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	page := repository.PageQuery{Limit: 20, WithTotal: true}
	found, total, err := repo.SearchByDayPeriod(ctx, "月", 1, "", 2026, "前期", page)
	if err != nil || total != 2 || len(found) != 2 || found[0].SourceRef != "ref-a" {
		t.Fatalf("search: %+v total=%d err=%v", found, total, err)
	}
	if _, err := db.Exec(`UPDATE courses SET discontinued_at=100 WHERE id=?`, first.ID); err != nil {
		t.Fatal(err)
	}
	sortBy, order := "messageCount", "DESC"
	courses, total, err := repo.ListCourses(ctx, repository.ListCoursesParam{Page: page, SortBy: &sortBy, SortOrder: &order})
	if err != nil || total != 2 || len(courses) != 2 {
		t.Fatalf("list total=%d err=%v", total, err)
	}
	if courses[0].ID != first.ID || courses[0].MessageCount != 2 || courses[0].SourceRef != "ref-a" || !courses[0].IsDiscontinued() || courses[1].ID != second.ID {
		t.Fatalf("counts multiplied by registrations or sync metadata lost: %+v", courses[0])
	}
	found, total, err = repo.SearchByDayPeriod(ctx, "月", 1, "", 2026, "前期", page)
	if err != nil || total != 1 || len(found) != 1 || found[0].ID != second.ID {
		t.Fatalf("retired course in search: total=%d err=%v", total, err)
	}
	order = "ASC"
	courses, _, err = repo.ListCourses(ctx, repository.ListCoursesParam{Page: page, SortBy: &sortBy, SortOrder: &order})
	if err != nil || len(courses) != 2 || courses[0].ID != second.ID {
		t.Fatalf("ascending sort: %v", err)
	}
}

func TestAdminPostsAndOwnersAfterAccountSplit(t *testing.T) {
	ddl := append([]string{}, userTablesDDL...)
	ddl = append(ddl,
		`CREATE TABLE posts(id BIGINT PRIMARY KEY,content TEXT,created_at BIGINT,updated_at BIGINT,user_id BIGINT,parent_id BIGINT NULL,deleted_at BIGINT NULL,reply_count INT)`,
		`CREATE TABLE room_users(room_id BIGINT,user_id BIGINT,role VARCHAR(50),created_at BIGINT)`)
	db, cleanup := throwawaySchemaDB(t, "space_admin_integration", ddl)
	defer cleanup()
	for _, stmt := range []string{
		`INSERT INTO users(id,status,created_at) VALUES(1,'active',1),(2,'deleted',1)`,
		`INSERT INTO user_accounts(user_id,account_id,name,email,hashed_password,role,updated_at) VALUES(1,'account1','実名','test@example.com','test','student',1)`,
		`INSERT INTO posts VALUES(1,'本文',1,1,1,NULL,NULL,0),(2,'削除済み著者の投稿',1,1,2,NULL,NULL,0)`,
		`INSERT INTO room_users VALUES(10,1,'owner',1),(20,2,'owner',1)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	posts, total, err := NewMySQLPostRepository(db).AdminListPosts(ctx, repository.AdminPostFilter{Keyword: "実名"}, repository.PageQuery{Limit: 20, WithTotal: true})
	if err != nil || total != 1 || len(posts) != 1 || posts[0].ID != 1 {
		t.Fatalf("name search after account split: total=%d err=%v", total, err)
	}
	posts, total, err = NewMySQLPostRepository(db).AdminListPosts(ctx, repository.AdminPostFilter{Keyword: "account1"}, repository.PageQuery{Limit: 20, WithTotal: true})
	if err != nil || total != 1 || len(posts) != 1 {
		t.Fatalf("account search: total=%d err=%v", total, err)
	}
	posts, total, err = NewMySQLPostRepository(db).AdminListPosts(ctx, repository.AdminPostFilter{}, repository.PageQuery{Limit: 20, WithTotal: true})
	if err != nil || total != 2 || len(posts) != 2 {
		t.Fatalf("deleted author hid admin posts: total=%d err=%v", total, err)
	}
	owners, err := NewMySQLRoomUserRepository(db).ListOwnersByRoomIDs(ctx, []int64{10, 20})
	if err != nil || len(owners) != 1 || owners[10] == nil || owners[10].Name != "実名" {
		t.Fatalf("owner lookup after account split: %+v err=%v", owners, err)
	}
}

// スレッドは授業のルームとは別のルームに書かれる。授業の発言数にはスレッドの中の発言も入れないと、
// スレッドで盛り上がっている授業が「ほとんど発言の無い授業」に見えてしまう。
func TestCourseMessageCountIncludesThreads(t *testing.T) {
	db, cleanup := courseSyncTestDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewMySQLCourseRepository(db)
	quiet, err := repo.SaveCourseWithRoom(ctx, repository.SaveCourseParam{DayOfWeek: "月", Period: 1, TeacherName: "先生", CourseName: "授業A", Year: 2026, Semester: "前期", DedupKey: "a", Source: model.CourseSourceManual})
	if err != nil {
		t.Fatal(err)
	}
	busy, err := repo.SaveCourseWithRoom(ctx, repository.SaveCourseParam{DayOfWeek: "月", Period: 1, TeacherName: "先生", CourseName: "授業B", Year: 2026, Semester: "前期", DedupKey: "b", Source: model.CourseSourceManual})
	if err != nil {
		t.Fatal(err)
	}
	// 授業A: 授業のルームに1件。授業B: 授業のルームには無く、2つのスレッドに合わせて3件。
	mustExec(t, db, `INSERT INTO rooms (id, name, type, created_at, updated_at, parent_room_id) VALUES (900, 't1', 'course', 1, 1, ?), (901, 't2', 'course', 1, 1, ?)`, busy.RoomID, busy.RoomID)
	mustExec(t, db, `INSERT INTO messages VALUES (1, ?), (2, 900), (3, 900), (4, 901)`, quiet.RoomID)

	sortBy, order := "messageCount", "DESC"
	courses, _, err := repo.ListCourses(ctx, repository.ListCoursesParam{Page: repository.PageQuery{Limit: 20}, SortBy: &sortBy, SortOrder: &order})
	if err != nil || len(courses) != 2 {
		t.Fatalf("list: %v", err)
	}
	if courses[0].ID != busy.ID || courses[0].MessageCount != 3 || courses[1].MessageCount != 1 {
		t.Fatalf("thread messages are not counted: %+v / %+v", courses[0], courses[1])
	}
	found, _, err := repo.SearchByDayPeriod(ctx, "月", 1, "授業B", 2026, "前期", repository.PageQuery{Limit: 20})
	if err != nil || len(found) != 1 || found[0].MessageCount != 3 {
		t.Fatalf("search does not count thread messages: %+v err=%v", found, err)
	}
}
