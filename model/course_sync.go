package model

import "time"

// CourseSnapshot は授業1コマぶんの「同期で比べる中身」。変更の前後や確認待ちの
// 内容として記録・表示に使う（courses の行そのものではない）。
type CourseSnapshot struct {
	CourseID    int64  `json:"courseId,omitempty"`
	SourceRef   string `json:"sourceRef"`
	Semester    string `json:"semester"`
	DayOfWeek   string `json:"dayOfWeek"`
	Period      int    `json:"period"`
	CourseName  string `json:"courseName"`
	TeacherName string `json:"teacherName"`
}

// SnapshotOf は授業の今の中身を CourseSnapshot にする。
func SnapshotOf(c *Course) CourseSnapshot {
	return CourseSnapshot{
		CourseID:    c.ID,
		SourceRef:   c.SourceRef,
		Semester:    c.Semester,
		DayOfWeek:   c.DayOfWeek,
		Period:      c.Period,
		CourseName:  c.CourseName,
		TeacherName: c.TeacherName,
	}
}

// CourseSyncChangeKind は同期1回の中で授業1件に起きたこと。
type CourseSyncChangeKind string

const (
	CourseSyncChangeCreated      CourseSyncChangeKind = "CREATED"
	CourseSyncChangeUpdated      CourseSyncChangeKind = "UPDATED"
	CourseSyncChangeDiscontinued CourseSyncChangeKind = "DISCONTINUED"
	CourseSyncChangeRestored     CourseSyncChangeKind = "RESTORED"
	CourseSyncChangeReview       CourseSyncChangeKind = "REVIEW"
)

// CourseSyncReviewKind は自動では判断せず管理者に回す変化の種類。
type CourseSyncReviewKind string

const (
	// CourseSyncReviewCodeReused は同じ講義コードのまま授業名と教員名が両方変わった。
	// 講義コードが別の授業に使い回された疑いがある。
	CourseSyncReviewCodeReused CourseSyncReviewKind = "CODE_REUSED"
	// CourseSyncReviewCodeReissued はある講義コードが消え、同じ授業名・教員名で
	// 新しい講義コードが現れた。講義コードが振り直された疑いがある。
	CourseSyncReviewCodeReissued CourseSyncReviewKind = "CODE_REISSUED"
	// CourseSyncReviewSlotAmbiguous は複数コマの授業でコマが変わり、どのコマが
	// どのコマに移ったのか1対1に決まらない。
	CourseSyncReviewSlotAmbiguous CourseSyncReviewKind = "SLOT_AMBIGUOUS"
)

// CourseSyncReviewStatus は確認の状態（106 の migration のコメント参照）。
type CourseSyncReviewStatus string

const (
	CourseSyncReviewPending   CourseSyncReviewStatus = "PENDING"
	CourseSyncReviewSame      CourseSyncReviewStatus = "SAME"
	CourseSyncReviewDifferent CourseSyncReviewStatus = "DIFFERENT"
	CourseSyncReviewIgnored   CourseSyncReviewStatus = "IGNORED"
	CourseSyncReviewApplied   CourseSyncReviewStatus = "APPLIED"
)

// CourseSyncReview は管理者の確認に回した変化1件。
type CourseSyncReview struct {
	ID           int64
	Year         int
	Kind         CourseSyncReviewKind
	Fingerprint  string
	Status       CourseSyncReviewStatus
	Message      string
	Existing     []CourseSnapshot
	Proposed     []CourseSnapshot
	FirstRunID   *int64
	AppliedRunID *int64
	ResolvedBy   *int64
	ResolvedAt   *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CourseSyncRun は同期1回ぶんの結果の集計。
type CourseSyncRun struct {
	ID          int64
	Year        int
	DryRun      bool
	TriggeredBy *int64
	// SiteTotal はシラバスサイトが報告した授業の件数、ListedRows は実際に読めた行数。
	// 一致しないときは取りこぼしがあり得るので、廃止の判定をしない。
	SiteTotal        int
	ListedRows       int
	UnidentifiedRows int
	Created          int
	Updated          int
	Discontinued     int
	Restored         int
	Reviews          int
	Unchanged        int
	// Unregistered はコマが変わった結果、時間割から外した登録の数（全授業の合計）。
	Unregistered int
	// DiscontinueSkippedReason は廃止の判定を見送った理由。空なら判定した。
	DiscontinueSkippedReason string
	StartedAt                time.Time
	FinishedAt               time.Time
}

// CourseSyncChange は同期1回の中で授業1件に起きたことの記録。
type CourseSyncChange struct {
	ID       int64
	RunID    int64
	Kind     CourseSyncChangeKind
	CourseID *int64
	ReviewID *int64
	// CourseName / TeacherName は一覧で見分けるための表示名（変更後、無ければ変更前）。
	CourseName  string
	TeacherName string
	// Detail は人が読む説明（例: "コマ: 水5 → 水4"）。
	Detail string
	Before *CourseSnapshot
	After  *CourseSnapshot
	// RegisteredCount はこの授業を時間割に登録している人数。
	RegisteredCount int
	// UnregisteredCount はコマが変わった結果、移った先のコマに別の授業を登録していたため
	// 時間割からこの授業を外した人数。時間割は1コマ1授業で、本人が選んで元から
	// そのコマに入れていた授業の方を残す。
	UnregisteredCount int
}
