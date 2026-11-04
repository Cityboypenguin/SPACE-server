package model

import "time"

// CourseChatImageSource は授業内チャットの画像の添付先。
type CourseChatImageSource string

const (
	CourseChatImageSourceMessage  CourseChatImageSource = "message"
	CourseChatImageSourceQuestion CourseChatImageSource = "question"
	CourseChatImageSourceAnswer   CourseChatImageSource = "answer"
)

// CourseChatImage は授業内チャットに上がった画像1枚（ADM-05。管理画面で一元管理する）。
// メッセージ・質問・回答の添付は別々の保存先に入っているので、まとめて見るための形。
type CourseChatImage struct {
	Media  Media
	Source CourseChatImageSource
	// TargetID はメッセージ・質問・回答のID（Source による）。
	TargetID   int64
	RoomID     int64
	CourseName string
	Uploader   CourseChatImageUploader
	// TargetDeleted は添付先のメッセージが削除済み（画面には出ないが、画像のファイルは残っている）。
	TargetDeleted bool
	// Position は添付の並び順。元に戻すときに同じ位置へ置く。
	Position int
}

// RemovedCourseChatImage は管理者が利用者の目から外した画像1枚。ファイルは非公開の置き場にある。
type RemovedCourseChatImage struct {
	MediaID    int64
	Source     CourseChatImageSource
	TargetID   int64
	Position   int
	RoomID     int64
	CourseName string
	Uploader   CourseChatImageUploader
	// RemovedKey は今のファイルの場所、OriginalKey は元に戻す先。
	RemovedKey  string
	OriginalKey string
	ContentType string
	Width       *int
	Height      *int
	CreatedAt   time.Time
	RemovedAt   time.Time
	RemovedBy   int64
	// Reported は通報の写しがこの画像を指している。一覧を返すときに入れる。
	Reported bool
}

type CourseChatImageUploader struct {
	ID        int64
	Name      string
	AccountID string
}
