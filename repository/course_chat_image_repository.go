package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// CourseChatImageFilter は一覧の絞り込み。nil は絞らない。
type CourseChatImageFilter struct {
	RoomID *int64
	Source *model.CourseChatImageSource
}

// CourseChatImageRepository は授業内チャットの画像（メッセージ・質問・回答の添付）を
// まとめて扱う管理用の口。
type CourseChatImageRepository interface {
	// ListCourseChatImages は新しい順。授業のルームの、画像の添付だけを返す。
	ListCourseChatImages(ctx context.Context, filter CourseChatImageFilter, limit, offset int) ([]*model.CourseChatImage, int, error)
	// GetCourseChatImage は mediaID が授業内チャットの画像ならそれを返す（そうでなければ nil）。
	GetCourseChatImage(ctx context.Context, mediaID int64) (*model.CourseChatImage, error)
	// DeleteCourseChatImage は画像（media 行。添付の紐付けは外部キーで一緒に消える）を消す。
	DeleteCourseChatImage(ctx context.Context, mediaID int64) error
}
