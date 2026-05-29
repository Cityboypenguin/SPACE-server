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
	TargetPost      ReportTargetType = "POST"      // 投稿
	TargetComment   ReportTargetType = "COMMENT"   // コメント
	TargetPromotion ReportTargetType = "PROMOTION" // 宣伝やそのコメント
	TargetCommunity ReportTargetType = "COMMUNITY" // コミュニティ
)

type Report struct {
	ID           string
	ReporterID   int64            // 通報したユーザーのID
	Reporter     User             // 通報したユーザーのドメインオブジェクト
	TargetType   ReportTargetType // "POST", "COMMUNITY" など
	TargetID     string           // 対象のエンティティID
	Reason       string           // スパム、嫌がらせなどの理由
	CustomReason *string          // 自由記述の詳細な理由（任意）
	Status       ReportStatus     // 現在の対応ステータス
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ReportSearchFilter struct {
	Status     *ReportStatus
	TargetType *ReportTargetType
	ReporterID *int64
}