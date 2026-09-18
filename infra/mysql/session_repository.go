package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLSessionRepository struct {
	DB *sql.DB
}

func NewMySQLSessionRepository(db *sql.DB) *MySQLSessionRepository {
	return &MySQLSessionRepository{DB: db}
}

func (r *MySQLSessionRepository) RecordSession(ctx context.Context, userID int64, durationSeconds int, pageViews []repository.PageViewInput) error {
	now := time.Now()
	date := now.Format("2006-01-02")
	nowUnix := now.Unix()

	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_session_summaries (user_id, date, session_count, total_duration_seconds, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			session_count = session_count + 1,
			total_duration_seconds = total_duration_seconds + VALUES(total_duration_seconds),
			updated_at = VALUES(updated_at)`,
		userID, date, durationSeconds, nowUnix, nowUnix)
	if err != nil {
		return err
	}

	if err := insertPageViews(ctx, tx, userID, date, nowUnix, pageViews); err != nil {
		return err
	}

	return tx.Commit()
}

// pageViewStatColumns は page_view_stats の1行ぶんの列数（プレースホルダの分割に使う）。
const pageViewStatColumns = 8

// insertPageViews は1セッションぶんのページビューを複数 VALUES の INSERT にまとめる。
//
// 以前は pageViews を for で回して1件ずつ ExecContext していた。1セッションで
// 数十画面を踏むのは普通なので、セッション送信1回でその数だけ往復していた
// （トランザクションの中なので正しさは保たれていたが、往復ぶんそのまま遅い）。
//
// ON DUPLICATE KEY UPDATE の意味は1件ずつのときと変わらない。MySQL は複数 VALUES
// でも行を順に処理するので、同じ画面が同じ配列に2回入っていても view_count は
// ちゃんと2増える（1件ずつ撃っていたときと同じ結果）。
//
// 分割するのは、ここだけ件数の上限がクライアント任せだから。投票の選択肢や
// 添付のように画面の作りで数十件に収まるものと違い、ページビューは滞在時間ぶん
// 溜めて送られてくる。プレースホルダ上限（65535）に当たると
// 「送るほど落ちる」になるので、inChunks に上限ぶんで切ってもらう。
func insertPageViews(ctx context.Context, db dbtx, userID int64, date string, nowUnix int64, pageViews []repository.PageViewInput) error {
	return inChunks(pageViews, pageViewStatColumns, func(chunk []repository.PageViewInput) error {
		args := make([]any, 0, len(chunk)*pageViewStatColumns)
		for _, pv := range chunk {
			args = append(args, userID, date, pv.Path, 1, pv.DurationSeconds, pv.MaxScrollDepth, nowUnix, nowUnix)
		}
		_, err := db.ExecContext(ctx, `
			INSERT INTO page_view_stats (user_id, date, page_path, view_count, total_duration_seconds, total_max_scroll_depth, created_at, updated_at)
			VALUES `+valuesPlaceholders(len(chunk), pageViewStatColumns)+`
			ON DUPLICATE KEY UPDATE
				view_count = view_count + 1,
				total_duration_seconds = total_duration_seconds + VALUES(total_duration_seconds),
				total_max_scroll_depth = total_max_scroll_depth + VALUES(total_max_scroll_depth),
				updated_at = VALUES(updated_at)`,
			args...)
		return err
	})
}
