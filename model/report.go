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
	TargetMessage   ReportTargetType = "MESSAGE"   // メッセージ（DM・コミュニティ・授業内チャット）
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
	// ReportedMessage は通報を受けた時点のメッセージ。TargetMessage のときだけ入る。
	ReportedMessage *ReportedMessage
}

// ReportedMessage は通報を受けた時点のメッセージの写し。
//
// 通報後にメッセージが編集・削除されても、利用者が改名・退会しても、対応する側は
// 通報された時点の内容を確認できる必要があるので、messages を参照せずに値を持つ。
// DB には JSON を暗号化して保存する（列を増やすと項目の追加のたびに移行が要るため）。
// 項目名を変えると保存済みの通報が読めなくなるので、変えるときは旧名も読めるようにする。
type ReportedMessage struct {
	MessageID int64  `json:"messageId"`
	RoomID    int64  `json:"roomId"`
	RoomType  string `json:"roomType"`
	RoomName  string `json:"roomName"`
	// AuthorID は投稿者の利用者ID。授業内チャットは匿名が前提なので nil にし、
	// 表示名も匿名ラベルだけを持つ。退会済みの投稿者は AuthorID のみで名前は空。
	AuthorID        *int64                 `json:"authorId"`
	AuthorName      string                 `json:"authorName"`
	AuthorAccountID string                 `json:"authorAccountId"`
	Content         string                 `json:"content"`
	Media           []ReportedMessageMedia `json:"media"`
	CreatedAt       time.Time              `json:"createdAt"`
	UpdatedAt       time.Time              `json:"updatedAt"`
}

// ReportedMessageMedia は通報を受けた時点の添付。
type ReportedMessageMedia struct {
	ID          int64     `json:"id"`
	StorageKey  string    `json:"storageKey"`
	ContentType string    `json:"contentType"`
	Width       *int      `json:"width,omitempty"`
	Height      *int      `json:"height,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

type ReportSearchFilter struct {
	Status     *ReportStatus
	TargetType *ReportTargetType
	ReporterID *int64
}
