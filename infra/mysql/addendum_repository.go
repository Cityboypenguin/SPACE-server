package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/messagecrypto"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.AddendumRepository = &MySQLAddendumRepository{}

type MySQLAddendumRepository struct {
	DB     *sql.DB
	cipher *messagecrypto.Cipher
}

// NewMySQLAddendumRepository の本文は、質問・回答の本文と同じ鍵で暗号化して保存する。
func NewMySQLAddendumRepository(db *sql.DB) (*MySQLAddendumRepository, error) {
	cipher, err := messagecrypto.New(os.Getenv("MESSAGE_ENCRYPTION_KEY"))
	if err != nil {
		return nil, err
	}
	return &MySQLAddendumRepository{DB: db, cipher: cipher}, nil
}

func (r *MySQLAddendumRepository) CreateQuestionAddendum(ctx context.Context, a *model.Addendum, max int) error {
	return r.create(ctx, "questions", "question_addenda", "question_id", a, max)
}

func (r *MySQLAddendumRepository) CreateAnswerAddendum(ctx context.Context, a *model.Addendum, max int) error {
	return r.create(ctx, "answers", "answer_addenda", "answer_id", a, max)
}

// create は、数の確認と追加を1つのトランザクションで行う。先に親（質問・回答）の行を
// ロックするので、同じ質問・回答への追記は直列になり、同時に足されても max を超えない
// （数えてから足す2段だと、同時に数えた分だけ超える。分離レベルに頼らず親の行で止める）。
// parent / table / col は呼び出し元の定数だけ（利用者の入力は入らない）。
func (r *MySQLAddendumRepository) create(ctx context.Context, parent, table, col string, a *model.Addendum, max int) (err error) {
	body, err := r.cipher.Encrypt(a.Body)
	if err != nil {
		return fmt.Errorf("encrypt addendum body: %w", err)
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var locked int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM `+parent+` WHERE id = ? FOR UPDATE`, a.TargetID).Scan(&locked); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE `+col+` = ?`, a.TargetID).Scan(&count); err != nil {
		return err
	}
	if count >= max {
		err = repository.ErrAddendumLimit
		return err
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO `+table+` (`+col+`, user_id, body, created_at) VALUES (?, ?, ?, ?)`,
		a.TargetID, a.UserID, body, a.CreatedAt.Unix())
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
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
		if a.Body, err = r.cipher.Decrypt(a.Body); err != nil {
			return nil, fmt.Errorf("decrypt addendum body: %w", err)
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
