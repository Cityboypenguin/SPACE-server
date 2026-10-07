package repository

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// CourseUpdateParam はシラバス同期で既存の授業1件を書き換える内容。
type CourseUpdateParam struct {
	ID          int64
	RoomID      int64
	Semester    string
	DayOfWeek   string
	Period      int
	CourseName  string
	TeacherName string
	SourceRef   string
	SourceName  string
	DedupKey    string
	// Restore は廃止の印を外す。
	Restore bool
	// RenameRoom は授業ルームの名前も CourseName に合わせる（ルーム名は作成時に
	// 授業名を写したもので、授業名が変わったら追従させる）。
	RenameRoom bool
}

// SlotConflict は同期で外す時間割の登録1件。
type SlotConflict struct {
	TimetableID int64
	UserID      int64
	// CourseID は外す（コマが変わって移ってきた）授業。
	CourseID int64
	// KeptCourseID / KeptCourseName は同じコマに残す授業（通知の文面に使う）。
	KeptCourseID   int64
	KeptCourseName string
}

// ListCourseSyncReviewsParam は確認一覧の絞り込み。nil は絞り込まない。
type ListCourseSyncReviewsParam struct {
	Year   *int
	Status *model.CourseSyncReviewStatus
	Page   PageQuery
}

// CourseSyncRepository はシラバス同期の書き込みと、その結果（実行・変更・確認）の記録。
// 書き込み系は TxManager.RunInTx の中で呼ぶこと（同期は全部入るか何も入らないか）。
type CourseSyncRepository interface {
	// ListSyncableCourses は year の授業のうち、シラバスから取り込んだもの
	// （source = senshu で source_ref がある。廃止済みも含む）を返す。
	ListSyncableCourses(ctx context.Context, year int) ([]*model.Course, error)
	UpdateCourses(ctx context.Context, params []CourseUpdateParam) error
	// DiscontinueCourses は廃止の印を付ける（行は消さない）。
	DiscontinueCourses(ctx context.Context, ids []int64, at time.Time) error
	// RetireCourses は廃止の印を付けたうえで照合から外す（source_ref を NULL に、
	// dedup_key を退避する）。講義コードが別の授業に使い回されたとき、同じ
	// dedup_key で新しい授業を作れるようにするため。
	RetireCourses(ctx context.Context, ids []int64, at time.Time) error
	// CountRegistrations は授業ごとの時間割の登録人数を返す（登録が無い授業は含まない）。
	CountRegistrations(ctx context.Context, courseIDs []int64) (map[int64]int, error)
	// FindSlotConflicts は movedCourseIDs（同期でコマや学期が変わった授業）の登録のうち、
	// 同じ人の時間割で同じ年度・重なる学期・同じコマに別の授業が入っているものを返す。
	// 時間割は1コマ1授業なので、返したものは外す（DeleteTimetableEntries）。
	// 残す方は「動いていない授業」。重なった授業がどちらも動いた授業なら、先に登録した方。
	// 既に変更後のコマが書かれている（UpdateCourses の後で呼ぶ）前提。
	FindSlotConflicts(ctx context.Context, movedCourseIDs []int64) ([]SlotConflict, error)
	DeleteTimetableEntries(ctx context.Context, ids []int64) error
	// Savepoint / RollbackToSavepoint はドライランのためのもの。授業への書き込みを
	// 本実行と同じ順に行い（重なりの判定まで本物にするため）、記録を残す前に巻き戻す。
	Savepoint(ctx context.Context, name string) error
	RollbackToSavepoint(ctx context.Context, name string) error

	ListReviewsByYear(ctx context.Context, year int) ([]*model.CourseSyncReview, error)
	// UpsertReview は fingerprint で確認を1件作るか、同じ状況が再び起きたなら
	// 内容を新しくして確認待ちに戻す。ID を返す。
	UpsertReview(ctx context.Context, review *model.CourseSyncReview) (int64, error)
	MarkReviewsApplied(ctx context.Context, ids []int64, runID int64, at time.Time) error
	ListReviews(ctx context.Context, param ListCourseSyncReviewsParam) ([]*model.CourseSyncReview, int, error)
	GetReview(ctx context.Context, id int64) (*model.CourseSyncReview, error)
	// ResolveReview は確認待ち（PENDING）の確認に管理者の判断を記録する。
	// 確認待ちでなければ false。
	ResolveReview(ctx context.Context, id int64, status model.CourseSyncReviewStatus, resolvedBy int64, at time.Time) (bool, error)

	SaveRun(ctx context.Context, run *model.CourseSyncRun) (int64, error)
	SaveChanges(ctx context.Context, runID int64, changes []*model.CourseSyncChange) error
	ListRuns(ctx context.Context, year *int, page PageQuery) ([]*model.CourseSyncRun, int, error)
	GetRun(ctx context.Context, id int64) (*model.CourseSyncRun, error)
	ListChanges(ctx context.Context, runID int64, kind *model.CourseSyncChangeKind, page PageQuery) ([]*model.CourseSyncChange, int, error)
}
