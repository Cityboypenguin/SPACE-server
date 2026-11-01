package mysql

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// 学生の授業検索（SearchByDayPeriod）が、message_count 列つきの行を読めること。
// 以前は列が足りず、検索のたびにスキャンエラーになっていた。
func TestSearchByDayPeriod_ReturnsMessageCount(t *testing.T) {
	db, cleanup := unreadTestDB(t)
	defer cleanup()
	// courses は unreadTestDB が year / semester だけで作っているので、検索が読む列を足す。
	mustExec(t, db, `ALTER TABLE courses
		ADD COLUMN day_of_week VARCHAR(10) NOT NULL DEFAULT '', ADD COLUMN period INT NOT NULL DEFAULT 0,
		ADD COLUMN teacher_name VARCHAR(255) NOT NULL DEFAULT '', ADD COLUMN course_name VARCHAR(255) NOT NULL DEFAULT '',
		ADD COLUMN dedup_key VARCHAR(255) NOT NULL DEFAULT '', ADD COLUMN created_at BIGINT NOT NULL DEFAULT 0,
		ADD COLUMN updated_at BIGINT NOT NULL DEFAULT 0`)
	mustExec(t, db, `INSERT INTO rooms (id, type) VALUES (1, 'course'), (2, 'course')`)
	mustExec(t, db, `INSERT INTO courses (room_id, day_of_week, period, teacher_name, course_name, year, semester, dedup_key, created_at, updated_at) VALUES
		(1, '月', 1, '先生A', '微分積分', 2026, '前期', 'a', 1, 1), (2, '月', 1, '先生B', '線形代数', 2026, '前期', 'b', 1, 1)`)
	insertMessage(t, db, 1, 10, 100)
	insertMessage(t, db, 1, 10, 101)

	repo := &MySQLCourseRepository{DB: db}
	items, total, err := repo.SearchByDayPeriod(context.Background(), "月", 1, "", 2026, "前期", repository.PageQuery{Limit: 10, WithTotal: true})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("total=%d len=%d, want 2", total, len(items))
	}
	if items[0].CourseName != "微分積分" || items[0].MessageCount != 2 || items[1].MessageCount != 0 {
		t.Errorf("items = %+v / %+v", items[0], items[1])
	}
}
