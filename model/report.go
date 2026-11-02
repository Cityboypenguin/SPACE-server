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
	TargetQuestion  ReportTargetType = "QUESTION"  // 授業の質問
	TargetAnswer    ReportTargetType = "ANSWER"    // 授業の質問への回答
	TargetPoll      ReportTargetType = "POLL"      // 授業の投票
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
	// ReportedContent は通報を受けた時点の内容。メッセージ・質問・回答・投票のときだけ入る。
	ReportedContent *ReportedContent
}

// ReportedContent は通報を受けた時点の内容の写し。
//
// 通報後に編集・削除されても、利用者が改名・退会しても、対応する側は通報された時点の
// 内容を確認できる必要があるので、元の行を参照せずに値を持つ。
// DB には JSON を暗号化して保存する（列を増やすと項目の追加のたびに移行が要るため）。
// 項目名を変えると保存済みの通報が読めなくなるので、変えるときは旧名も読めるようにする。
// メッセージだけが対象だった時期の通報があるため、ContentID の JSON 名は messageId のまま。
type ReportedContent struct {
	ContentID int64  `json:"messageId"`
	RoomID    int64  `json:"roomId"`
	RoomType  string `json:"roomType"`
	RoomName  string `json:"roomName"`
	// AuthorID は投稿者の利用者ID。退会済みの投稿者は AuthorID のみで名前は空。
	// 授業内チャットが匿名表示だった時期の通報は nil で、表示名は匿名ラベルになっている。
	AuthorID        *int64 `json:"authorId"`
	AuthorName      string `json:"authorName"`
	AuthorAccountID string `json:"authorAccountId"`
	// Content はメッセージ・質問・回答の本文、または投票の設問。
	Content string                 `json:"content"`
	Media   []ReportedContentMedia `json:"media"`
	// StampID はスタンプだけのメッセージのスタンプのID。Content は空になる。
	StampID *string `json:"stampId,omitempty"`
	// Addenda は質問・回答への追記（古い順）。通報の時点までに足されたもの。
	Addenda []string `json:"addenda,omitempty"`
	// ParentBody は回答が答えている質問の本文。回答だけでは読み取れない文脈を残す。
	ParentBody *string `json:"parentBody,omitempty"`
	// PollOptions は投票の選択肢。誰がどれに投票したかは持たない。
	PollOptions []string  `json:"pollOptions,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// ReportedContentMedia は通報を受けた時点の添付。
type ReportedContentMedia struct {
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
