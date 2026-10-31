package model

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
}

type CourseChatImageUploader struct {
	ID        int64
	Name      string
	AccountID string
}
