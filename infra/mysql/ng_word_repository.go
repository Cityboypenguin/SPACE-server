package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.NGWordRepository = &MySQLNGWordRepository{}

type MySQLNGWordRepository struct {
	DB *sql.DB
}

func NewMySQLNGWordRepository(db *sql.DB) repository.NGWordRepository {
	return &MySQLNGWordRepository{DB: db}
}

func (r *MySQLNGWordRepository) ListNGWords(ctx context.Context) ([]*model.NGWord, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id, pattern, match_type, created_at FROM ng_words ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.NGWord
	for rows.Next() {
		var w model.NGWord
		var matchType string
		var createdAt int64
		if err := rows.Scan(&w.ID, &w.Pattern, &matchType, &createdAt); err != nil {
			return nil, err
		}
		w.MatchType = model.NGWordMatchType(matchType)
		w.CreatedAt = time.Unix(createdAt, 0)
		out = append(out, &w)
	}
	return out, rows.Err()
}

func (r *MySQLNGWordRepository) CreateNGWord(ctx context.Context, w *model.NGWord, max int) error {
	// 数の確認と登録を1つの文にする（確認してから登録する2段だと、同時に登録された分だけ上限を超える）。
	res, err := r.DB.ExecContext(ctx,
		`INSERT INTO ng_words (pattern, match_type, created_at)
		 SELECT ?, ?, ? FROM DUAL WHERE (SELECT COUNT(*) FROM ng_words) < ?`,
		w.Pattern, string(w.MatchType), w.CreatedAt.Unix(), max)
	if err != nil {
		if isDuplicateKeyError(err) {
			return repository.ErrDuplicateKey
		}
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return repository.ErrNGWordLimit
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	w.ID = id
	return nil
}

func (r *MySQLNGWordRepository) DeleteNGWord(ctx context.Context, id int64) (bool, error) {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM ng_words WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *MySQLNGWordRepository) RecordNGWordHit(ctx context.Context, hit *model.NGWordHit) error {
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO ng_word_hits (ng_word_id, pattern, user_id, room_id, source, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		hit.NGWordID, hit.Pattern, hit.UserID, hit.RoomID, string(hit.Source), hit.CreatedAt.Unix())
	return err
}

func (r *MySQLNGWordRepository) NGWordHitStats(ctx context.Context, topN int) (*model.NGWordHitStats, error) {
	stats := &model.NGWordHitStats{}
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM ng_word_hits`).Scan(&stats.Total); err != nil {
		return nil, fmt.Errorf("failed to count ng word hits: %w", err)
	}

	wordRows, err := r.DB.QueryContext(ctx,
		`SELECT pattern, COUNT(*) AS c FROM ng_word_hits GROUP BY pattern ORDER BY c DESC, pattern ASC LIMIT ?`, topN)
	if err != nil {
		return nil, err
	}
	defer wordRows.Close()
	for wordRows.Next() {
		var c model.NGWordHitCount
		if err := wordRows.Scan(&c.Pattern, &c.Count); err != nil {
			return nil, err
		}
		stats.ByWord = append(stats.ByWord, c)
	}
	if err := wordRows.Err(); err != nil {
		return nil, err
	}

	userRows, err := r.DB.QueryContext(ctx, `
		SELECT h.user_id, u.name, u.account_id, COUNT(*) AS c
		FROM ng_word_hits h
		INNER JOIN user_accounts u ON u.user_id = h.user_id
		GROUP BY h.user_id, u.name, u.account_id
		ORDER BY c DESC, h.user_id ASC
		LIMIT ?`, topN)
	if err != nil {
		return nil, err
	}
	defer userRows.Close()
	for userRows.Next() {
		var c model.NGWordUserHitCount
		if err := userRows.Scan(&c.UserID, &c.Name, &c.AccountID, &c.Count); err != nil {
			return nil, err
		}
		stats.ByUser = append(stats.ByUser, c)
	}
	return stats, userRows.Err()
}
