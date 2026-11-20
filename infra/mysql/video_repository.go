package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLVideoRepository struct {
	DB *sql.DB
}

func NewMySQLVideoRepository(db *sql.DB) repository.VideoRepository {
	return &MySQLVideoRepository{DB: db}
}

// videoColumns は動画1本を組み立てる列。最後に「付いている先が削除済みか」を足して使う。
const videoColumns = `m.id, m.uploader_user_id, m.storage_key, m.width, m.height, m.created_at,
	va.thumbnail_key, va.size_bytes, va.duration_ms, va.fps_milli, va.has_audio,
	va.removed_at, va.removed_reason, va.purged_at`

// videoParentJoins は付いている先（投稿・メッセージ）を左結合する。
// どちらか一方にしか付かないので、片方は NULL になる。
const videoParentJoins = `
	LEFT JOIN post_videos pv ON pv.media_id = va.media_id
	LEFT JOIN posts p ON p.id = pv.post_id
	LEFT JOIN message_videos mv ON mv.media_id = va.media_id
	LEFT JOIN messages msg ON msg.id = mv.message_id`

const videoParentDeleted = `(p.deleted_at IS NOT NULL OR msg.deleted_at IS NOT NULL)`

// scanVideo は videoColumns と、続く extra の列を読む。
func scanVideo(row rowScanner, extra ...any) (*model.Video, error) {
	var (
		v                   model.Video
		width, height       sql.NullInt64
		createdAt           int64
		removedAt, purgedAt sql.NullInt64
		removedReason       sql.NullString
		hasAudio            bool
	)
	dest := []any{&v.MediaID, &v.UploaderUserID, &v.StorageKey, &width, &height, &createdAt,
		&v.ThumbnailKey, &v.SizeBytes, &v.DurationMs, &v.FPSMilli, &hasAudio,
		&removedAt, &removedReason, &purgedAt}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return nil, err
	}
	v.Width = int(width.Int64)
	v.Height = int(height.Int64)
	v.HasAudio = hasAudio
	v.CreatedAt = time.Unix(createdAt, 0)
	if removedAt.Valid {
		t := time.Unix(removedAt.Int64, 0)
		v.RemovedAt = &t
	}
	if removedReason.Valid {
		reason := model.VideoRemovedReason(removedReason.String)
		v.RemovedReason = &reason
	}
	if purgedAt.Valid {
		t := time.Unix(purgedAt.Int64, 0)
		v.PurgedAt = &t
	}
	return &v, nil
}

func (r *MySQLVideoRepository) CreateVideo(ctx context.Context, v *model.Video) error {
	db := extractDB(ctx, r.DB)
	result, err := db.ExecContext(ctx, `
		INSERT INTO media (uploader_user_id, storage_key, content_type, width, height, created_at)
		VALUES (?, ?, 'video/mp4', ?, ?, ?)`,
		v.UploaderUserID, v.StorageKey, v.Width, v.Height, v.CreatedAt.Unix())
	if err != nil {
		return err
	}
	if v.MediaID, err = result.LastInsertId(); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO video_assets (media_id, thumbnail_key, size_bytes, duration_ms, fps_milli, has_audio)
		VALUES (?, ?, ?, ?, ?, ?)`,
		v.MediaID, v.ThumbnailKey, v.SizeBytes, v.DurationMs, v.FPSMilli, v.HasAudio)
	return err
}

func (r *MySQLVideoRepository) AttachVideoToPost(ctx context.Context, postID, mediaID int64) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`INSERT INTO post_videos (post_id, media_id) VALUES (?, ?)`, postID, mediaID)
	return err
}

func (r *MySQLVideoRepository) AttachVideoToMessage(ctx context.Context, messageID, mediaID int64) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`INSERT INTO message_videos (message_id, media_id) VALUES (?, ?)`, messageID, mediaID)
	return err
}

func (r *MySQLVideoRepository) MarkVideoRemoved(ctx context.Context, mediaID int64, reason model.VideoRemovedReason, by int64, at time.Time) error {
	// 先に外した理由を上書きしない（投稿者が外した後に管理者が外しても、記録は最初のまま）。
	_, err := extractDB(ctx, r.DB).ExecContext(ctx, `
		UPDATE video_assets SET removed_at = ?, removed_reason = ?, removed_by = ?
		WHERE media_id = ? AND removed_at IS NULL`,
		at.Unix(), string(reason), by, mediaID)
	return err
}

func (r *MySQLVideoRepository) ListVideosByPostIDs(ctx context.Context, postIDs []int64) (map[int64]*model.Video, error) {
	return r.listByParent(ctx, postIDs, `
		SELECT `+videoColumns+`, p.deleted_at IS NOT NULL, pv.post_id
		FROM post_videos pv
		JOIN posts p ON p.id = pv.post_id
		JOIN video_assets va ON va.media_id = pv.media_id
		JOIN media m ON m.id = va.media_id
		WHERE pv.post_id IN (%s)`)
}

func (r *MySQLVideoRepository) ListVideosByMessageIDs(ctx context.Context, messageIDs []int64) (map[int64]*model.Video, error) {
	return r.listByParent(ctx, messageIDs, `
		SELECT `+videoColumns+`, msg.deleted_at IS NOT NULL, mv.message_id
		FROM message_videos mv
		JOIN messages msg ON msg.id = mv.message_id
		JOIN video_assets va ON va.media_id = mv.media_id
		JOIN media m ON m.id = va.media_id
		WHERE mv.message_id IN (%s)`)
}

func (r *MySQLVideoRepository) listByParent(ctx context.Context, ids []int64, queryFmt string) (map[int64]*model.Video, error) {
	result := make(map[int64]*model.Video, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := extractDB(ctx, r.DB).QueryContext(ctx, fmt.Sprintf(queryFmt, inPlaceholders(len(ids))), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var parentID int64
		var deleted bool
		v, err := scanVideo(rows, &deleted, &parentID)
		if err != nil {
			return nil, err
		}
		v.ParentDeleted = deleted
		result[parentID] = v
	}
	return result, rows.Err()
}

func (r *MySQLVideoRepository) GetVideo(ctx context.Context, mediaID int64) (*model.Video, *model.VideoParent, error) {
	row := extractDB(ctx, r.DB).QueryRowContext(ctx, `
		SELECT `+videoColumns+`, `+videoParentDeleted+`, pv.post_id, mv.message_id
		FROM video_assets va
		JOIN media m ON m.id = va.media_id`+videoParentJoins+`
		WHERE va.media_id = ?`, mediaID)
	var deleted bool
	var postID, messageID sql.NullInt64
	v, err := scanVideo(row, &deleted, &postID, &messageID)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	v.ParentDeleted = deleted
	var parent *model.VideoParent
	switch {
	case postID.Valid:
		parent = &model.VideoParent{Kind: model.VideoParentPost, ID: postID.Int64}
	case messageID.Valid:
		parent = &model.VideoParent{Kind: model.VideoParentMessage, ID: messageID.Int64}
	}
	return v, parent, nil
}

// openReportOn は「その投稿・メッセージに未対応・確認中の通報がある」。
// 通報の target_id は数値IDの文字列で入っている。古い行の status は小文字のことがある。
//
// 数値IDを文字列にして比べるとき、両辺とも CAST して接続の照合順序にそろえる。
// 片側だけ CAST すると、列（user_reports は utf8mb4_unicode_ci）と接続の照合順序が
// 違うため "Illegal mix of collations" で失敗する。数値のまま比べると target_id 側が
// 数値に変換され、INSERT ... SELECT では数値でない行が strict モードのエラーになる。
const openReportOn = `EXISTS (SELECT 1 FROM user_reports r
	WHERE r.target_type = ? AND CAST(r.target_id AS CHAR) = CAST(%s AS CHAR)
	  AND UPPER(r.status) IN ('PENDING', 'REVIEWING'))`

func (r *MySQLVideoRepository) ListVideosToPurge(ctx context.Context, q repository.VideoPurgeQuery) ([]*model.Video, error) {
	query := `
		SELECT ` + videoColumns + `, ` + videoParentDeleted + `
		FROM video_assets va
		JOIN media m ON m.id = va.media_id` + videoParentJoins + `
		WHERE va.purged_at IS NULL
		  AND (va.purge_next_attempt_at IS NULL OR va.purge_next_attempt_at <= ?)
		  AND (
		        m.created_at < ?
		     OR va.removed_at < ?
		     OR p.deleted_at < ?
		     OR msg.deleted_at < ?
		     OR (pv.post_id IS NULL AND mv.message_id IS NULL)
		  )
		  AND NOT ` + fmt.Sprintf(openReportOn, "pv.post_id") + `
		  AND NOT ` + fmt.Sprintf(openReportOn, "mv.message_id") + `
		ORDER BY m.created_at, m.id
		LIMIT ?`
	removed := q.RemovedBefore.Unix()
	rows, err := extractDB(ctx, r.DB).QueryContext(ctx, query,
		q.Now.Unix(), q.ExpiredBefore.Unix(), removed, removed, removed,
		string(model.TargetPost), string(model.TargetMessage), q.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Video
	for rows.Next() {
		var deleted bool
		v, err := scanVideo(rows, &deleted)
		if err != nil {
			return nil, err
		}
		v.ParentDeleted = deleted
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *MySQLVideoRepository) MarkVideoPurged(ctx context.Context, mediaID int64, at time.Time) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx, `
		UPDATE video_assets
		SET purged_at = ?, purge_next_attempt_at = NULL, purge_last_error = NULL
		WHERE media_id = ? AND purged_at IS NULL`, at.Unix(), mediaID)
	return err
}

// maxPurgeErrorLen は記録するエラー文の長さ（列は VARCHAR(255)）。
const maxPurgeErrorLen = 255

// truncatePurgeError は記録するエラー文を列に収める。文字の途中で切らないよう1文字ずつ詰める。
func truncatePurgeError(message string) string {
	for len(message) > maxPurgeErrorLen || !utf8.ValidString(message) {
		_, size := utf8.DecodeLastRuneInString(message)
		message = message[:len(message)-max(size, 1)]
	}
	return message
}

func (r *MySQLVideoRepository) RecordVideoPurgeFailure(ctx context.Context, mediaID int64, message string, nextAttemptAt time.Time) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx, `
		UPDATE video_assets
		SET purge_attempts = purge_attempts + 1, purge_next_attempt_at = ?, purge_last_error = ?
		WHERE media_id = ? AND purged_at IS NULL`, nextAttemptAt.Unix(), truncatePurgeError(message), mediaID)
	return err
}

// RetainReportedMessageVideos は退会のトランザクションの中で呼ばれるので extractDB を通す。
// 実体を消し終えた動画（purged_at あり）は残すものが無いので対象にしない。
func (r *MySQLVideoRepository) RetainReportedMessageVideos(ctx context.Context, userID int64, now time.Time) ([]string, error) {
	db := extractDB(ctx, r.DB)
	if _, err := db.ExecContext(ctx, `
		INSERT IGNORE INTO reported_video_retentions
			(media_id, storage_key, thumbnail_key, size_bytes, message_id, created_at)
		SELECT m.id, m.storage_key, va.thumbnail_key, va.size_bytes, mv.message_id, ?
		FROM media m
		JOIN video_assets va ON va.media_id = m.id
		JOIN message_videos mv ON mv.media_id = m.id
		WHERE m.uploader_user_id = ? AND va.purged_at IS NULL
		  AND `+fmt.Sprintf(openReportOn, "mv.message_id"),
		now.Unix(), userID, string(model.TargetMessage)); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT rv.storage_key, rv.thumbnail_key
		FROM reported_video_retentions rv
		JOIN media m ON m.id = rv.media_id
		WHERE m.uploader_user_id = ? AND rv.purged_at IS NULL
		ORDER BY rv.media_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key, thumb string
		if err := rows.Scan(&key, &thumb); err != nil {
			return nil, err
		}
		keys = append(keys, key, thumb)
	}
	return keys, rows.Err()
}

const retentionColumns = `media_id, storage_key, thumbnail_key, size_bytes, message_id, created_at, purged_at`

func scanRetention(row rowScanner) (repository.ReportedVideoRetention, error) {
	var (
		rv        repository.ReportedVideoRetention
		createdAt int64
		purgedAt  sql.NullInt64
	)
	if err := row.Scan(&rv.MediaID, &rv.StorageKey, &rv.ThumbnailKey, &rv.SizeBytes, &rv.MessageID, &createdAt, &purgedAt); err != nil {
		return rv, err
	}
	rv.CreatedAt = time.Unix(createdAt, 0)
	if purgedAt.Valid {
		t := time.Unix(purgedAt.Int64, 0)
		rv.PurgedAt = &t
	}
	return rv, nil
}

func (r *MySQLVideoRepository) ListRetentionsToPurge(ctx context.Context, now time.Time, limit int) ([]repository.ReportedVideoRetention, error) {
	rows, err := extractDB(ctx, r.DB).QueryContext(ctx, `
		SELECT `+retentionColumns+`
		FROM reported_video_retentions
		WHERE purged_at IS NULL
		  AND (purge_next_attempt_at IS NULL OR purge_next_attempt_at <= ?)
		  AND NOT `+fmt.Sprintf(openReportOn, "message_id")+`
		ORDER BY created_at, media_id
		LIMIT ?`, now.Unix(), string(model.TargetMessage), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []repository.ReportedVideoRetention
	for rows.Next() {
		rv, err := scanRetention(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rv)
	}
	return out, rows.Err()
}

func (r *MySQLVideoRepository) MarkRetentionPurged(ctx context.Context, mediaID int64, at time.Time) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx, `
		UPDATE reported_video_retentions
		SET purged_at = ?, purge_next_attempt_at = NULL, purge_last_error = NULL
		WHERE media_id = ? AND purged_at IS NULL`, at.Unix(), mediaID)
	return err
}

func (r *MySQLVideoRepository) RecordRetentionPurgeFailure(ctx context.Context, mediaID int64, message string, nextAttemptAt time.Time) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx, `
		UPDATE reported_video_retentions
		SET purge_attempts = purge_attempts + 1, purge_next_attempt_at = ?, purge_last_error = ?
		WHERE media_id = ? AND purged_at IS NULL`, nextAttemptAt.Unix(), truncatePurgeError(message), mediaID)
	return err
}

func (r *MySQLVideoRepository) GetRetention(ctx context.Context, mediaID int64) (*repository.ReportedVideoRetention, error) {
	rv, err := scanRetention(extractDB(ctx, r.DB).QueryRowContext(ctx,
		`SELECT `+retentionColumns+` FROM reported_video_retentions WHERE media_id = ?`, mediaID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rv, nil
}

func (r *MySQLVideoRepository) usage(ctx context.Context, query string, args ...any) (repository.VideoUsage, error) {
	var u repository.VideoUsage
	err := extractDB(ctx, r.DB).QueryRowContext(ctx, query, args...).Scan(&u.Count, &u.Bytes)
	return u, err
}

// StoredVideoUsage は退会のあと通報のために残している動画も数える（容量を使っているため）。
func (r *MySQLVideoRepository) StoredVideoUsage(ctx context.Context) (repository.VideoUsage, error) {
	return r.usage(ctx, `
		SELECT COUNT(*), COALESCE(SUM(size_bytes), 0) FROM (
			SELECT size_bytes FROM video_assets WHERE purged_at IS NULL
			UNION ALL
			SELECT size_bytes FROM reported_video_retentions WHERE purged_at IS NULL
		) s`)
}

func (r *MySQLVideoRepository) UserVideoUsageSince(ctx context.Context, userID int64, since time.Time) (repository.VideoUsage, error) {
	return r.usage(ctx, `
		SELECT COUNT(*), COALESCE(SUM(va.size_bytes), 0)
		FROM video_assets va JOIN media m ON m.id = va.media_id
		WHERE m.uploader_user_id = ? AND m.created_at >= ?`, userID, since.Unix())
}

func (r *MySQLVideoRepository) VideosExpiringBefore(ctx context.Context, cutoff time.Time) (repository.VideoUsage, error) {
	return r.usage(ctx, `
		SELECT COUNT(*), COALESCE(SUM(va.size_bytes), 0)
		FROM video_assets va
		JOIN media m ON m.id = va.media_id`+videoParentJoins+`
		WHERE va.purged_at IS NULL AND va.removed_at IS NULL
		  AND NOT `+videoParentDeleted+`
		  AND m.created_at < ?`, cutoff.Unix())
}

func (r *MySQLVideoRepository) PurgeFailingCount(ctx context.Context) (int64, error) {
	var n int64
	err := extractDB(ctx, r.DB).QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM video_assets WHERE purged_at IS NULL AND purge_attempts > 0) +
			(SELECT COUNT(*) FROM reported_video_retentions WHERE purged_at IS NULL AND purge_attempts > 0)`).Scan(&n)
	return n, err
}

func (r *MySQLVideoRepository) AddVideoDailyStats(ctx context.Context, d repository.VideoDailyStats) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx, `
		INSERT INTO video_daily_stats
			(day, uploads, upload_bytes, upload_failures, playback_requests, playback_bytes, purged, purge_failures)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			uploads = uploads + VALUES(uploads),
			upload_bytes = upload_bytes + VALUES(upload_bytes),
			upload_failures = upload_failures + VALUES(upload_failures),
			playback_requests = playback_requests + VALUES(playback_requests),
			playback_bytes = playback_bytes + VALUES(playback_bytes),
			purged = purged + VALUES(purged),
			purge_failures = purge_failures + VALUES(purge_failures)`,
		d.Day, d.Uploads, d.UploadBytes, d.UploadFailures, d.PlaybackRequests, d.PlaybackBytes, d.Purged, d.PurgeFailures)
	return err
}

func (r *MySQLVideoRepository) ListVideoDailyStats(ctx context.Context, fromDay, toDay int) ([]repository.VideoDailyStats, error) {
	rows, err := extractDB(ctx, r.DB).QueryContext(ctx, `
		SELECT day, uploads, upload_bytes, upload_failures, playback_requests, playback_bytes, purged, purge_failures
		FROM video_daily_stats WHERE day BETWEEN ? AND ? ORDER BY day`, fromDay, toDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []repository.VideoDailyStats
	for rows.Next() {
		var d repository.VideoDailyStats
		if err := rows.Scan(&d.Day, &d.Uploads, &d.UploadBytes, &d.UploadFailures, &d.PlaybackRequests, &d.PlaybackBytes, &d.Purged, &d.PurgeFailures); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
