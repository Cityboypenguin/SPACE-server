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

var _ repository.QuestionRepository = &MySQLQuestionRepository{}

type MySQLQuestionRepository struct {
	DB     *sql.DB
	cipher *messagecrypto.Cipher
}

func NewMySQLQuestionRepository(db *sql.DB) (*MySQLQuestionRepository, error) {
	cipher, err := messagecrypto.New(os.Getenv("MESSAGE_ENCRYPTION_KEY"))
	if err != nil {
		return nil, err
	}
	return &MySQLQuestionRepository{DB: db, cipher: cipher}, nil
}

func (r *MySQLQuestionRepository) SaveQuestion(ctx context.Context, q *model.Question) error {
	body, err := r.cipher.Encrypt(q.Body)
	if err != nil {
		return fmt.Errorf("encrypt question body: %w", err)
	}

	now := time.Now()
	nowUnix := now.Unix()
	result, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`INSERT INTO questions (room_id, asker_user_id, author_role, body, is_answered, created_at, updated_at)
		 VALUES (?, ?, ?, ?, FALSE, ?, ?)`,
		q.RoomID, q.AskerUserID, q.AuthorRole, body, nowUnix, nowUnix,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	q.ID = id
	q.IsAnswered = false
	q.CreatedAt = now
	q.UpdatedAt = now
	return nil
}

func (r *MySQLQuestionRepository) GetQuestionByID(ctx context.Context, id int64) (*model.Question, error) {
	row := extractDB(ctx, r.DB).QueryRowContext(ctx,
		`SELECT id, room_id, asker_user_id, author_role, body, is_answered, best_answer_id, created_at, updated_at
		 FROM questions WHERE id = ?`, id)
	return r.scanQuestion(row)
}

func (r *MySQLQuestionRepository) ListQuestionsByRoomID(ctx context.Context, roomID int64, limit, offset int) ([]*model.Question, int, error) {
	var total int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM questions WHERE room_id = ?`, roomID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, room_id, asker_user_id, author_role, body, is_answered, best_answer_id, created_at, updated_at
		 FROM questions WHERE room_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		roomID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []*model.Question
	for rows.Next() {
		q, err := r.scanQuestion(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, q)
	}
	return list, total, rows.Err()
}

func (r *MySQLQuestionRepository) SetBestAnswer(ctx context.Context, questionID, answerID, askerUserID int64) (bool, error) {
	result, err := r.DB.ExecContext(ctx,
		`UPDATE questions SET best_answer_id = ?, is_answered = TRUE, updated_at = ? WHERE id = ? AND asker_user_id = ?`,
		answerID, time.Now().Unix(), questionID, askerUserID,
	)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

type questionScanner interface {
	Scan(dest ...any) error
}

func (r *MySQLQuestionRepository) scanQuestion(row questionScanner) (*model.Question, error) {
	var q model.Question
	var bestAnswerID sql.NullInt64
	var createdAt, updatedAt int64
	if err := row.Scan(&q.ID, &q.RoomID, &q.AskerUserID, &q.AuthorRole, &q.Body, &q.IsAnswered, &bestAnswerID, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if bestAnswerID.Valid {
		q.BestAnswerID = &bestAnswerID.Int64
	}
	q.CreatedAt = time.Unix(createdAt, 0)
	q.UpdatedAt = time.Unix(updatedAt, 0)

	body, err := r.cipher.Decrypt(q.Body)
	if err != nil {
		return nil, fmt.Errorf("decrypt question body: %w", err)
	}
	q.Body = body

	return &q, nil
}
