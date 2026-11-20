package repository

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// VideoWriter は動画の登録と紐付け。投稿・メッセージの保存と同じトランザクションで呼ぶ。
type VideoWriter interface {
	// CreateVideo は media の行と video_assets の行を作り、MediaID を埋めて返す。
	CreateVideo(ctx context.Context, v *model.Video) error
	AttachVideoToPost(ctx context.Context, postID, mediaID int64) error
	AttachVideoToMessage(ctx context.Context, messageID, mediaID int64) error
	// MarkVideoRemoved は動画を期限より前に見えなくする。既に外したものはそのまま。
	MarkVideoRemoved(ctx context.Context, mediaID int64, reason model.VideoRemovedReason, by int64, at time.Time) error
}

// VideoReader は動画を引く（DataLoader 用の一括取得を含む）。
// 外したもの・期限が過ぎたもの・実体を消したものも返す。見せ方は呼び出し側が決める。
type VideoReader interface {
	ListVideosByPostIDs(ctx context.Context, postIDs []int64) (map[int64]*model.Video, error)
	ListVideosByMessageIDs(ctx context.Context, messageIDs []int64) (map[int64]*model.Video, error)
	// GetVideo は動画と、それが付いている先を返す。無ければ nil。
	GetVideo(ctx context.Context, mediaID int64) (*model.Video, *model.VideoParent, error)
}

// VideoPurgeQuery は実体を消す対象の条件。
type VideoPurgeQuery struct {
	Now time.Time
	// ExpiredBefore より前に登録された動画は期限＋猶予を過ぎている。
	ExpiredBefore time.Time
	// RemovedBefore より前に外された動画（投稿者・管理者・投稿の削除）は猶予を過ぎている。
	RemovedBefore time.Time
	Limit         int
}

// VideoPurger は日次の削除が使う口。
type VideoPurger interface {
	// ListVideosToPurge は実体を消してよい動画を返す。
	// 未対応・確認中の通報が付いている投稿・メッセージの動画は含めない
	// （通報への対応が終わるまで残す）。やり直しの時刻が来ていないものも含めない。
	ListVideosToPurge(ctx context.Context, q VideoPurgeQuery) ([]*model.Video, error)
	MarkVideoPurged(ctx context.Context, mediaID int64, at time.Time) error
	RecordVideoPurgeFailure(ctx context.Context, mediaID int64, message string, nextAttemptAt time.Time) error
}

// VideoUsage は量の集計。
type VideoUsage struct {
	Count int64
	Bytes int64
}

// VideoDailyStats は video_daily_stats の1日ぶん。Day は JST の yyyymmdd。
type VideoDailyStats struct {
	Day              int
	Uploads          int64
	UploadBytes      int64
	UploadFailures   int64
	PlaybackRequests int64
	PlaybackBytes    int64
	Purged           int64
	PurgeFailures    int64
}

// VideoStatsRepository は量の把握と上限の判定に使う。
type VideoStatsRepository interface {
	// StoredVideoUsage は実体がまだ残っている動画（purged_at が NULL）の本数と合計容量。
	StoredVideoUsage(ctx context.Context) (VideoUsage, error)
	// UserVideoUsageSince はその利用者が since 以降に登録した動画の本数と合計容量。
	UserVideoUsageSince(ctx context.Context, userID int64, since time.Time) (VideoUsage, error)
	// VideosExpiringBefore は、外されておらず実体も残っていて、cutoff より前に登録された動画
	// （＝期限が cutoff 基準で来ている動画）の本数と合計容量。保存期間の変更の影響を見るのに使う。
	VideosExpiringBefore(ctx context.Context, cutoff time.Time) (VideoUsage, error)
	// PurgeBacklog は実体を消す前の段階にある動画のうち、削除に失敗しているものの本数。
	PurgeFailingCount(ctx context.Context) (int64, error)
	// AddVideoDailyStats は day の集計に delta を足す（行が無ければ作る）。
	AddVideoDailyStats(ctx context.Context, delta VideoDailyStats) error
	ListVideoDailyStats(ctx context.Context, fromDay, toDay int) ([]VideoDailyStats, error)
}

// ReportedVideoRetention は退会で消える動画のうち、通報への対応が終わるまで残したもの
// （reported_video_retentions の1行）。
type ReportedVideoRetention struct {
	MediaID      int64
	StorageKey   string
	ThumbnailKey string
	SizeBytes    int64
	MessageID    int64
	CreatedAt    time.Time
	PurgedAt     *time.Time
}

// VideoRetentionRepository は退会のときに通報中の動画を残し、対応が終わったら消すための口。
type VideoRetentionRepository interface {
	// RetainReportedMessageVideos は userID が投稿したメッセージの動画のうち、未対応・確認中の
	// 通報が付いているものを残す記録を作り、残すファイルのキー（動画とサムネイル）を返す。
	// 退会のトランザクションの中で、media の行が消える前に呼ぶ。
	RetainReportedMessageVideos(ctx context.Context, userID int64, now time.Time) ([]string, error)
	// ListRetentionsToPurge は通報への対応が終わった（未対応・確認中の通報が無くなった）もの。
	ListRetentionsToPurge(ctx context.Context, now time.Time, limit int) ([]ReportedVideoRetention, error)
	MarkRetentionPurged(ctx context.Context, mediaID int64, at time.Time) error
	RecordRetentionPurgeFailure(ctx context.Context, mediaID int64, message string, nextAttemptAt time.Time) error
	// GetRetention は残した記録を返す。無ければ nil。
	GetRetention(ctx context.Context, mediaID int64) (*ReportedVideoRetention, error)
}

// VideoRepository は上記をすべて束ねた口。infra の実装と DI が使う。
type VideoRepository interface {
	VideoWriter
	VideoReader
	VideoPurger
	VideoStatsRepository
	VideoRetentionRepository
}
