package model

import "time"

type ReportStatus string

const (
	StatusPending   ReportStatus = "PENDING"   // 未対応
	StatusReviewing ReportStatus = "REVIEWING" // 確認中
	StatusResolved  ReportStatus = "RESOLVED"  // 解決済み
	StatusDismissed ReportStatus = "DISMISSED" // 却下（問題なし）
)

type ReportTargetType string

const (
	TargetPost      ReportTargetType = "POST"      // 投稿（コメントも親を持つ投稿として扱われるため対象はPOSTに統一）
	TargetUser      ReportTargetType = "USER"      // ユーザー
	TargetCommunity ReportTargetType = "COMMUNITY" // コミュニティ
)

type Report struct {
	ID           string
	ReporterID   int64
	Reporter     User
	TargetType   ReportTargetType
	TargetID     string
	Reason       string
	CustomReason *string
	Status       ReportStatus
	CreatedAt    time.Time
	UpdatedAt    time.Time
	PostID       *int64  `json:"postId"`
	PostContent  *string `json:"postContent"`
}

type ReportSearchFilter struct {
	Status     *ReportStatus
	TargetType *ReportTargetType
	ReporterID *int64
}
