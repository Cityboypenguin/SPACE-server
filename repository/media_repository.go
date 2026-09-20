package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type MediaRepository interface {
	CreateMedia(ctx context.Context, m *model.Media) error

	// CreateMediaBatch は media 行をまとめて1本の INSERT で作り、採番されたIDを
	// 各要素の ID へ書き戻す。並び順は渡した順のまま。
	//
	// 添付の保存は以前「1件ごとに CreateMedia + Create<親>Media」の2往復を
	// していたので、4枚付けると8往復していた。一括版は2往復で終わる。
	CreateMediaBatch(ctx context.Context, ms []*model.Media) error

	// Create<親>MediaBatch は添付の紐付けを1本の INSERT で作る。position は
	// startPosition から mediaIDs の順に1つずつ増やして振る（投稿の編集で
	// 既存の添付の後ろに足す場合に、続きの番号から始められるようにするため）。
	//
	// 1件ずつ作る版は置いていない。呼び出し側は必ず「添付の配列」を持っており、
	// 1件ずつの口を残すと再び for の中で1件ずつ呼ぶ書き方が戻ってくるため。
	CreatePostMediaBatch(ctx context.Context, postID int64, mediaIDs []int64, startPosition int) error
	CreateMessageMediaBatch(ctx context.Context, messageID int64, mediaIDs []int64, startPosition int) error
	CreateQuestionMediaBatch(ctx context.Context, questionID int64, mediaIDs []int64, startPosition int) error
	CreateAnswerMediaBatch(ctx context.Context, answerID int64, mediaIDs []int64, startPosition int) error
	ListByPostID(ctx context.Context, postID int64) ([]*model.Media, error)
	ListByPostIDs(ctx context.Context, postIDs []int64) (map[int64][]*model.Media, error)
	ListByMessageIDs(ctx context.Context, messageIDs []int64) (map[int64][]*model.Media, error)
	ListByQuestionIDs(ctx context.Context, questionIDs []int64) (map[int64][]*model.Media, error)
	ListByAnswerIDs(ctx context.Context, answerIDs []int64) (map[int64][]*model.Media, error)
	DeleteMediaByIDAndUserID(ctx context.Context, mediaID, userID int64) error
	// DeleteQuestionMedia は questionID に添付されている mediaID を削除する（紐付けは
	// ON DELETE CASCADE で消える）。他の質問・投稿のメディアは対象にしない。
	DeleteQuestionMedia(ctx context.Context, questionID, mediaID int64) error
	// DeleteAnswerMedia は answerID に添付されている mediaID を削除する（紐付けは
	// ON DELETE CASCADE で消える）。他の回答・投稿のメディアは対象にしない。
	DeleteAnswerMedia(ctx context.Context, answerID, mediaID int64) error
	GetMaxPostMediaPosition(ctx context.Context, postID int64) (int, error)
	// ListImagesMissingDimensions は寸法が未取得の画像メディアを ID 昇順で返す。
	ListImagesMissingDimensions(ctx context.Context, limit, offset int) ([]*model.Media, error)
	// SetMediaDimensionsIfUnset は寸法が未設定のときだけ記録する。
	// 値はクライアントの観測値なので、いちど入った値を上書きさせない。
	SetMediaDimensionsIfUnset(ctx context.Context, mediaID int64, width, height int) error
}
