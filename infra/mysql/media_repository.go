package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLMediaRepository struct {
	DB *sql.DB
}

func NewMySQLMediaRepository(db *sql.DB) repository.MediaRepository {
	return &MySQLMediaRepository{DB: db}
}

func (r *MySQLMediaRepository) CreateMedia(ctx context.Context, m *model.Media) error {
	db := extractDB(ctx, r.DB)
	query := `
		INSERT INTO media (uploader_user_id, storage_key, content_type, width, height, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	result, err := db.ExecContext(ctx, query,
		m.UploaderUserID, m.StorageKey, m.ContentType, m.Width, m.Height, m.CreatedAt.Unix(),
	)
	if err != nil {
		return err
	}
	m.ID, err = result.LastInsertId()
	return err
}

// mediaInsertColumns / mediaLinkColumns は一括 INSERT の1行あたりの列数（分割の単位）。
const (
	mediaInsertColumns = 6
	mediaLinkColumns   = 3
)

func (r *MySQLMediaRepository) CreateMediaBatch(ctx context.Context, ms []*model.Media) error {
	db := extractDB(ctx, r.DB)
	return inChunks(ms, mediaInsertColumns, func(chunk []*model.Media) error {
		args := make([]any, 0, len(chunk)*mediaInsertColumns)
		for _, m := range chunk {
			args = append(args, m.UploaderUserID, m.StorageKey, m.ContentType, m.Width, m.Height, m.CreatedAt.Unix())
		}
		result, err := db.ExecContext(ctx, `
			INSERT INTO media (uploader_user_id, storage_key, content_type, width, height, created_at)
			VALUES `+valuesPlaceholders(len(chunk), mediaInsertColumns), args...)
		if err != nil {
			return err
		}
		// 行数が事前に分かる複数 VALUES の INSERT では、InnoDB は AUTO_INCREMENT を
		// 連番でまとめて確保する（notifications の SaveBatch も同じ前提に乗っている）。
		// なので先頭IDから順に振り直してよい。
		firstID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		for i, m := range chunk {
			m.ID = firstID + int64(i)
		}
		return nil
	})
}

// createMediaLinks は添付の紐付けを1本の INSERT で作る。
// 親の種類ごとに表が違うだけで中身は同じなので、読み取り側の listByParentIDs と
// 同じく表名と外部キー列名だけを引数にして1箇所に集約する。
func (r *MySQLMediaRepository) createMediaLinks(ctx context.Context, joinTable, fkColumn string, parentID int64, mediaIDs []int64, startPosition int) error {
	db := extractDB(ctx, r.DB)
	offset := 0
	return inChunks(mediaIDs, mediaLinkColumns, func(chunk []int64) error {
		args := make([]any, 0, len(chunk)*mediaLinkColumns)
		for i, mediaID := range chunk {
			args = append(args, parentID, mediaID, startPosition+offset+i)
		}
		offset += len(chunk)
		_, err := db.ExecContext(ctx, fmt.Sprintf(
			`INSERT INTO %s (%s, media_id, position) VALUES %s`,
			joinTable, fkColumn, valuesPlaceholders(len(chunk), mediaLinkColumns)), args...)
		return err
	})
}

func (r *MySQLMediaRepository) CreatePostMediaBatch(ctx context.Context, postID int64, mediaIDs []int64, startPosition int) error {
	return r.createMediaLinks(ctx, "post_media", "post_id", postID, mediaIDs, startPosition)
}

func (r *MySQLMediaRepository) CreateMessageMediaBatch(ctx context.Context, messageID int64, mediaIDs []int64, startPosition int) error {
	return r.createMediaLinks(ctx, "message_media", "message_id", messageID, mediaIDs, startPosition)
}

func (r *MySQLMediaRepository) CreateQuestionMediaBatch(ctx context.Context, questionID int64, mediaIDs []int64, startPosition int) error {
	return r.createMediaLinks(ctx, "question_media", "question_id", questionID, mediaIDs, startPosition)
}

func (r *MySQLMediaRepository) CreateAnswerMediaBatch(ctx context.Context, answerID int64, mediaIDs []int64, startPosition int) error {
	return r.createMediaLinks(ctx, "answer_media", "answer_id", answerID, mediaIDs, startPosition)
}

func (r *MySQLMediaRepository) ListByPostID(ctx context.Context, postID int64) ([]*model.Media, error) {
	query := `
		SELECT m.id, m.uploader_user_id, m.storage_key, m.content_type, m.width, m.height, m.created_at
		FROM media m
		JOIN post_media pm ON pm.media_id = m.id
		WHERE pm.post_id = ?
		ORDER BY pm.position ASC
	`
	rows, err := r.DB.QueryContext(ctx, query, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.Media
	for rows.Next() {
		var m model.Media
		var createdAt int64
		if err := rows.Scan(&m.ID, &m.UploaderUserID, &m.StorageKey, &m.ContentType, &m.Width, &m.Height, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt = time.Unix(createdAt, 0)
		result = append(result, &m)
	}
	return result, rows.Err()
}

func (r *MySQLMediaRepository) DeleteMediaByIDAndUserID(ctx context.Context, mediaID, userID int64) error {
	query := `DELETE FROM media WHERE id = ? AND uploader_user_id = ?`
	_, err := r.DB.ExecContext(ctx, query, mediaID, userID)
	return err
}

func (r *MySQLMediaRepository) DeleteQuestionMedia(ctx context.Context, questionID, mediaID int64) error {
	db := extractDB(ctx, r.DB)
	query := `
		DELETE m FROM media m
		JOIN question_media qm ON qm.media_id = m.id
		WHERE qm.question_id = ? AND m.id = ?
	`
	_, err := db.ExecContext(ctx, query, questionID, mediaID)
	return err
}

func (r *MySQLMediaRepository) DeleteAnswerMedia(ctx context.Context, answerID, mediaID int64) error {
	db := extractDB(ctx, r.DB)
	query := `
		DELETE m FROM media m
		JOIN answer_media am ON am.media_id = m.id
		WHERE am.answer_id = ? AND m.id = ?
	`
	_, err := db.ExecContext(ctx, query, answerID, mediaID)
	return err
}

func (r *MySQLMediaRepository) GetMaxPostMediaPosition(ctx context.Context, postID int64) (int, error) {
	query := `SELECT MAX(position) FROM post_media WHERE post_id = ?`

	var maxPos sql.NullInt32
	err := r.DB.QueryRowContext(ctx, query, postID).Scan(&maxPos)
	if err != nil {
		return 0, err
	}

	if maxPos.Valid {
		return int(maxPos.Int32), nil
	}
	return -1, nil
}

func (r *MySQLMediaRepository) listByParentIDs(ctx context.Context, ids []int64, joinTable, fkColumn string) (map[int64][]*model.Media, error) {
	result := make(map[int64][]*model.Media)
	if len(ids) == 0 {
		return result, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT m.id, m.uploader_user_id, m.storage_key, m.content_type, m.width, m.height, m.created_at, jt.%s
		FROM media m
		JOIN %s jt ON jt.media_id = m.id
		WHERE jt.%s IN (%s)
		ORDER BY jt.%s, jt.position ASC
	`, fkColumn, joinTable, fkColumn, placeholders, fkColumn)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var m model.Media
		var createdAt, parentID int64
		if err := rows.Scan(&m.ID, &m.UploaderUserID, &m.StorageKey, &m.ContentType, &m.Width, &m.Height, &createdAt, &parentID); err != nil {
			return nil, err
		}
		m.CreatedAt = time.Unix(createdAt, 0)
		result[parentID] = append(result[parentID], &m)
	}
	return result, rows.Err()
}

func (r *MySQLMediaRepository) ListByMessageIDs(ctx context.Context, messageIDs []int64) (map[int64][]*model.Media, error) {
	return r.listByParentIDs(ctx, messageIDs, "message_media", "message_id")
}

func (r *MySQLMediaRepository) ListByPostIDs(ctx context.Context, postIDs []int64) (map[int64][]*model.Media, error) {
	return r.listByParentIDs(ctx, postIDs, "post_media", "post_id")
}

func (r *MySQLMediaRepository) ListByQuestionIDs(ctx context.Context, questionIDs []int64) (map[int64][]*model.Media, error) {
	return r.listByParentIDs(ctx, questionIDs, "question_media", "question_id")
}

func (r *MySQLMediaRepository) ListByAnswerIDs(ctx context.Context, answerIDs []int64) (map[int64][]*model.Media, error) {
	return r.listByParentIDs(ctx, answerIDs, "answer_media", "answer_id")
}

func (r *MySQLMediaRepository) SetMediaDimensionsIfUnset(ctx context.Context, mediaID int64, width, height int) error {
	db := extractDB(ctx, r.DB)
	// 条件を WHERE に置くことで、確認と更新の間に他のリクエストが書き込む余地をなくす。
	// すでに入っていれば 0 行更新になるだけで、エラーにはしない。
	query := `
		UPDATE media
		SET width = ?, height = ?
		WHERE id = ? AND width IS NULL AND height IS NULL
	`
	_, err := db.ExecContext(ctx, query, width, height, mediaID)
	return err
}

func (r *MySQLMediaRepository) ListImagesMissingDimensions(ctx context.Context, limit, offset int) ([]*model.Media, error) {
	query := `
		SELECT id, uploader_user_id, storage_key, content_type, width, height, created_at
		FROM media
		WHERE content_type LIKE 'image/%'
		  AND (width IS NULL OR height IS NULL)
		ORDER BY id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := r.DB.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.Media
	for rows.Next() {
		var m model.Media
		var createdAt int64
		if err := rows.Scan(&m.ID, &m.UploaderUserID, &m.StorageKey, &m.ContentType, &m.Width, &m.Height, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt = time.Unix(createdAt, 0)
		result = append(result, &m)
	}
	return result, rows.Err()
}
