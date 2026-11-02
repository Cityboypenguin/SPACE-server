package repository

import (
	"context"
	"errors"
	"time"

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
	// RemoveCourseChatImage は記録を removed_course_chat_images へ移し、media 行を消す
	// （添付の紐付けは外部キーで一緒に消える）。
	RemoveCourseChatImage(ctx context.Context, removed *model.RemovedCourseChatImage) error
	// ListRemovedCourseChatImages は外した画像を、外した順の新しいものから返す。
	ListRemovedCourseChatImages(ctx context.Context, filter CourseChatImageFilter, limit, offset int) ([]*model.RemovedCourseChatImage, int, error)
	// GetRemovedCourseChatImage は無ければ nil を返す。
	GetRemovedCourseChatImage(ctx context.Context, mediaID int64) (*model.RemovedCourseChatImage, error)
	// ListRemovedCourseChatImagesBefore は removed_at が before より古いものを media_id の順に返す。
	// afterMediaID より後から limit 件（消さずに残す行があっても先へ進めるように）。
	ListRemovedCourseChatImagesBefore(ctx context.Context, before time.Time, afterMediaID int64, limit int) ([]*model.RemovedCourseChatImage, error)
	// CheckRestorable は元に戻す先の投稿と投稿者が残っているかを確かめる。
	CheckRestorable(ctx context.Context, removed *model.RemovedCourseChatImage) error
	// RestoreCourseChatImage は media 行と添付の紐付けを作り直し、外した記録を消す。
	RestoreCourseChatImage(ctx context.Context, removed *model.RemovedCourseChatImage) error
	// DeleteRemovedCourseChatImage は外した記録を消す。
	DeleteRemovedCourseChatImage(ctx context.Context, mediaID int64) error
}

// 元に戻せない理由。
var (
	ErrRestoreTargetGone   = errors.New("the post of the removed image no longer exists")
	ErrRestoreUploaderGone = errors.New("the uploader of the removed image has withdrawn")
)
