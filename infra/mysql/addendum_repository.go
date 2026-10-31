package mysql

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.AddendumRepository = &MySQLAddendumRepository{}

type MySQLAddendumRepository struct {
	DB *sql.DB
}

func NewMySQLAddendumRepository(db *sql.DB) repository.AddendumRepository {
	return &MySQLAddendumRepository{DB: db}
}

func (r *MySQLAddendumRepository) CreateQuestionAddendum(ctx context.Context, a *model.Addendum, max int) error {
	return r.create(ctx, "question_addenda", "question_id", a, max)
}

func (r *MySQLAddendumRepository) CreateAnswerAddendum(ctx context.Context, a *model.Addendum, max int) error {
	return r.create(ctx, "answer_addenda", "answer_id", a, max)
}

// create は、数の確認と追加を1つの文にする（確認してから足す2段だと、同時に足された分だけ上限を超える）。
// table / col は呼び出し元の定数だけ（利用者の入力は入らない）。
func (r *MySQLAddendumRepository) create(ctx context.Context, table, col string, a *model.Addendum, max int) error {
	res, err := r.DB.ExecContext(ctx,
		`INSERT INTO `+table+` (`+col+`, user_id, body, created_at)
		 SELECT ?, ?, ?, ? FROM DUAL WHERE (SELECT COUNT(*) FROM `+table+` WHERE `+col+` = ?) < ?`,
		a.TargetID, a.UserID, a.Body, a.CreatedAt.Unix(), a.TargetID, max)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return repository.ErrAddendumLimit
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	a.ID = id
	return nil
}

func (r *MySQLAddendumRepository) ListQuestionAddenda(ctx context.Context, questionIDs []int64) (map[int64][]*model.Addendum, error) {
	return r.list(ctx, "question_addenda", "question_id", questionIDs)
}

func (r *MySQLAddendumRepository) ListAnswerAddenda(ctx context.Context, answerIDs []int64) (map[int64][]*model.Addendum, error) {
	return r.list(ctx, "answer_addenda", "answer_id", answerIDs)
}

func (r *MySQLAddendumRepository) list(ctx context.Context, table, col string, ids []int64) (map[int64][]*model.Addendum, error) {
	out := make(map[int64][]*model.Addendum)
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, `+col+`, user_id, body, created_at FROM `+table+
			` WHERE `+col+` IN (`+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`) ORDER BY id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a model.Addendum
		var createdAt int64
		if err := rows.Scan(&a.ID, &a.TargetID, &a.UserID, &a.Body, &createdAt); err != nil {
			return nil, err
		}
		a.CreatedAt = time.Unix(createdAt, 0)
		out[a.TargetID] = append(out[a.TargetID], &a)
	}
	return out, rows.Err()
}

func (r *MySQLAddendumRepository) ListAnswererIDs(ctx context.Context, questionID int64) ([]int64, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT DISTINCT author_user_id FROM answers WHERE question_id = ? ORDER BY author_user_id`, questionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
