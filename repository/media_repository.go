package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type MediaRepository interface {
	CreateMedia(ctx context.Context, m *model.Media) error
	CreatePostMedia(ctx context.Context, postID, mediaID int64, position int) error
	CreateMessageMedia(ctx context.Context, messageID, mediaID int64, position int) error
	CreateQuestionMedia(ctx context.Context, questionID, mediaID int64, position int) error
	CreateAnswerMedia(ctx context.Context, answerID, mediaID int64, position int) error
	ListByPostID(ctx context.Context, postID int64) ([]*model.Media, error)
	ListByPostIDs(ctx context.Context, postIDs []int64) (map[int64][]*model.Media, error)
	ListByMessageIDs(ctx context.Context, messageIDs []int64) (map[int64][]*model.Media, error)
	ListByQuestionIDs(ctx context.Context, questionIDs []int64) (map[int64][]*model.Media, error)
	ListByAnswerIDs(ctx context.Context, answerIDs []int64) (map[int64][]*model.Media, error)
	DeleteMediaByIDAndUserID(ctx context.Context, mediaID, userID int64) error
	GetMaxPostMediaPosition(ctx context.Context, postID int64) (int, error)
	// ListImagesMissingDimensions は寸法が未取得の画像メディアを ID 昇順で返す。
	ListImagesMissingDimensions(ctx context.Context, limit, offset int) ([]*model.Media, error)
	// SetMediaDimensionsIfUnset は寸法が未設定のときだけ記録する。
	// 値はクライアントの観測値なので、いちど入った値を上書きさせない。
	SetMediaDimensionsIfUnset(ctx context.Context, mediaID int64, width, height int) error
}
