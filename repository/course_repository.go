package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// SaveCourseParam holds the fields needed to create a Course together with its Room.
type SaveCourseParam struct {
	DayOfWeek   string
	Period      int
	TeacherName string
	CourseName  string
	Year        int
	Semester    string
	DedupKey    string
}

// ListCoursesParam holds the optional filters for ListCourses (admin course listing).
// A nil Year/Semester/DayOfWeek means "no filter on this field"; an empty Keyword means
// no course-name/teacher-name filter.
type ListCoursesParam struct {
	Year      *int
	Semester  *string
	DayOfWeek *string
	Keyword   string
	// Page は窓と「total を数えるか」。他の一覧と同じ PageQuery に揃えてある
	// （以前は Limit/Offset の2フィールドだった）。
	Page PageQuery
}

type CourseRepository interface {
	// SaveCourseWithRoom creates a Room (type=course) and a Course in a single transaction,
	// mirroring CommunityRepository.SaveCommunityWithRoom. It does not add any room_users row:
	// course rooms are readable/writable by any authenticated student, not membership-gated.
	SaveCourseWithRoom(ctx context.Context, param SaveCourseParam) (*model.Course, error)
	// SaveCoursesWithRooms は SaveCourseWithRoom の一括版。取り込みのように
	// 数百〜数千件をまとめて作る経路のためにある（1件ずつ呼ぶとトランザクションと
	// INSERT が件数ぶん並び、シラバス1年ぶんの取り込みで数千往復になっていた）。
	//
	// 意味は SaveCourseWithRoom と同じで、courses 1行につき rooms 1行を作り、
	// room_users には触らない。渡した順と同じ順で返す。
	// DedupKey の重複確認はしない（呼び出し側が FindExistingDedupKeys で
	// 落としてから渡すこと）。
	SaveCoursesWithRooms(ctx context.Context, params []SaveCourseParam) ([]*model.Course, error)
	FindByDedupKey(ctx context.Context, dedupKey string) (*model.Course, error)
	// FindExistingDedupKeys は渡した dedup_key のうち、既に courses に在るものだけを
	// true で返す。取り込みの存在確認を1件ずつの FindByDedupKey から1本の IN 句へ
	// まとめるためのもの。
	//
	// ListDedupKeysByYear と違い年で絞らない。取り込み対象そのものを問い合わせるので、
	// 年をまたいだ dedup_key（unique index はテーブル全体に張ってある）も正しく
	// 「既に在る」と判定できる。
	FindExistingDedupKeys(ctx context.Context, dedupKeys []string) (map[string]bool, error)
	GetCourseByID(ctx context.Context, id int64) (*model.Course, error)
	GetCourseByRoomID(ctx context.Context, roomID int64) (*model.Course, error)
	SearchByDayPeriod(ctx context.Context, dayOfWeek string, period int, keyword string, year int, semester string, q PageQuery) ([]*model.Course, int, error)
	ListCourses(ctx context.Context, param ListCoursesParam) ([]*model.Course, int, error)
	// ListDistinctYears returns every year present in courses, newest first, so the
	// admin course-management screen can offer a year picker backed by actual data
	// instead of an arbitrary free-typed number.
	ListDistinctYears(ctx context.Context) ([]int, error)
	// ListDedupKeysByYear returns every dedup_key already stored for the given
	// year, so a re-scrape of a year that's already been imported can skip
	// re-deriving anything (e.g. scraper-side duplicate/campus disambiguation)
	// for courses whose import is going to be a no-op anyway.
	ListDedupKeysByYear(ctx context.Context, year int) (map[string]bool, error)
}
