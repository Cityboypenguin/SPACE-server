package model

import "time"

// VideoProbe は受け入れ時に動画ファイルの中身から読んだ実測値（internal/upload.InspectMP4）。
// クライアントの申告は使わない。
type VideoProbe struct {
	SizeBytes  int64
	DurationMs int64
	Width      int
	Height     int
	// FPSMilli は平均フレームレートの1000倍。
	FPSMilli int64
	HasAudio bool
}

// VideoInput は投稿・メッセージに付ける動画1本ぶんのアップロード済みキー。
// サムネイルは端末が動画から切り出した JPEG で、動画と同じ期限で消す。
type VideoInput struct {
	StorageKey   string
	ThumbnailKey string
}

// VideoRemovedReason は動画を期限より前に見えなくした理由。
type VideoRemovedReason string

const (
	// VideoRemovedByAuthor は投稿者が編集で外した。
	VideoRemovedByAuthor VideoRemovedReason = "author"
	// VideoRemovedByAdmin は管理者が外した。
	VideoRemovedByAdmin VideoRemovedReason = "admin"
)

// Video は投稿・メッセージに付いた動画1本。media の行（StorageKey など）と
// video_assets の行（実測値と後片付けの状態）を合わせたもの。
//
// 期限（保存期間）は持たない。期限は「登録した時刻 + その時点の保存期間の設定」で
// 毎回計算する。期限を行に書くと、保存期間を変えたときに既存の動画へ効かせるために
// 全行を書き換えることになり、書き換えの途中で失敗すると動画ごとに期限がばらつく。
type Video struct {
	MediaID        int64
	UploaderUserID int64
	StorageKey     string
	ThumbnailKey   string
	SizeBytes      int64
	DurationMs     int64
	Width          int
	Height         int
	FPSMilli       int64
	HasAudio       bool
	CreatedAt      time.Time
	// RemovedAt は期限より前に見えなくした時刻（投稿者が外した・管理者が外した）。
	RemovedAt     *time.Time
	RemovedReason *VideoRemovedReason
	// ParentDeleted は付いている投稿・メッセージが削除されているか。
	ParentDeleted bool
	// PurgedAt は実体（動画とサムネイル）を消し終えた時刻。
	PurgedAt *time.Time
}

// VideoStatus は利用者から見た動画の状態。
type VideoStatus string

const (
	// VideoAvailable は再生できる。
	VideoAvailable VideoStatus = "AVAILABLE"
	// VideoExpired は保存期間が過ぎた。実体がまだ残っていても再生させない。
	VideoExpired VideoStatus = "EXPIRED"
	// VideoRemoved は期限より前に外された（投稿者・管理者・投稿の削除）。
	VideoRemoved VideoStatus = "REMOVED"
)

// ExpiresAt は保存期間 retention で計算した期限。
func (v *Video) ExpiresAt(retention time.Duration) time.Time {
	return v.CreatedAt.Add(retention)
}

// Status は now の時点の状態。外されたものは期限より優先して REMOVED になる。
func (v *Video) Status(now time.Time, retention time.Duration) VideoStatus {
	if v.RemovedAt != nil || v.ParentDeleted {
		return VideoRemoved
	}
	if v.PurgedAt != nil || !now.Before(v.ExpiresAt(retention)) {
		return VideoExpired
	}
	return VideoAvailable
}

// VideoParentKind は動画が付いている先の種類。
type VideoParentKind string

const (
	VideoParentPost    VideoParentKind = "post"
	VideoParentMessage VideoParentKind = "message"
)

// VideoParent は動画が付いている先。再生の可否（その投稿・メッセージを見てよいか）を
// 決めるのに使う。
type VideoParent struct {
	Kind VideoParentKind
	ID   int64
}
