package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/messagecrypto"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.AnswerRepository = &MySQLAnswerRepository{}

type MySQLAnswerRepository struct {
	DB     *sql.DB
	cipher *messagecrypto.Cipher
}

func NewMySQLAnswerRepository(db *sql.DB) (*MySQLAnswerRepository, error) {
	cipher, err := messagecrypto.New(os.Getenv("MESSAGE_ENCRYPTION_KEY"))
	if err != nil {
		return nil, err
	}
	return &MySQLAnswerRepository{DB: db, cipher: cipher}, nil
}

func (r *MySQLAnswerRepository) SaveAnswer(ctx context.Context, a *model.Answer) error {
	body, err := r.cipher.Encrypt(a.Body)
	if err != nil {
		return fmt.Errorf("encrypt answer body: %w", err)
	}

	now := time.Now()
	nowUnix := now.Unix()
	result, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`INSERT INTO answers (question_id, author_user_id, author_role, body, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		a.QuestionID, a.AuthorUserID, a.AuthorRole, body, nowUnix, nowUnix,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	a.ID = id
	a.CreatedAt = now
	a.UpdatedAt = now
	return nil
}

func (r *MySQLAnswerRepository) GetAnswerByID(ctx context.Context, id int64) (*model.Answer, error) {
	row := extractDB(ctx, r.DB).QueryRowContext(ctx,
		`SELECT id, question_id, author_user_id, author_role, body, created_at, updated_at FROM answers WHERE id = ?`, id)
	return r.scanAnswer(row)
}

func (r *MySQLAnswerRepository) ListAnswersByQuestionID(ctx context.Context, questionID int64) ([]*model.Answer, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, question_id, author_user_id, author_role, body, created_at, updated_at
		 FROM answers WHERE question_id = ? ORDER BY created_at ASC`,
		questionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*model.Answer
	for rows.Next() {
		a, err := r.scanAnswer(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

type answerScanner interface {
	Scan(dest ...any) error
}

func (r *MySQLAnswerRepository) scanAnswer(row answerScanner) (*model.Answer, error) {
	var a model.Answer
	var createdAt, updatedAt int64
	if err := row.Scan(&a.ID, &a.QuestionID, &a.AuthorUserID, &a.AuthorRole, &a.Body, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	a.CreatedAt = time.Unix(createdAt, 0)
	a.UpdatedAt = time.Unix(updatedAt, 0)

	body, err := r.cipher.Decrypt(a.Body)
	if err != nil {
		return nil, fmt.Errorf("decrypt answer body: %w", err)
	}
	a.Body = body

	return &a, nil
}
