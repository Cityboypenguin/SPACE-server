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

func (r *MySQLAnswerRepository) GetAnswerWithLikesByID(ctx context.Context, id, viewerUserID int64) (*repository.AnswerWithLikes, error) {
	row := extractDB(ctx, r.DB).QueryRowContext(ctx, `
		SELECT a.id, a.question_id, a.author_user_id, a.author_role, a.body, a.created_at, a.updated_at,
		       COUNT(al.id) AS like_count,
		       SUM(CASE WHEN al.user_id = ? THEN 1 ELSE 0 END) AS my_like_count
		FROM answers a
		LEFT JOIN answer_likes al ON al.answer_id = a.id
		WHERE a.id = ?
		GROUP BY a.id, a.question_id, a.author_user_id, a.author_role, a.body, a.created_at, a.updated_at
	`, viewerUserID, id)
	return r.scanAnswerWithLikes(row)
}

func (r *MySQLAnswerRepository) ListAnswersWithLikesByQuestionID(ctx context.Context, questionID, viewerUserID int64, limit, offset int) ([]*repository.AnswerWithLikes, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT a.id, a.question_id, a.author_user_id, a.author_role, a.body, a.created_at, a.updated_at,
		       COUNT(al.id) AS like_count,
		       SUM(CASE WHEN al.user_id = ? THEN 1 ELSE 0 END) AS my_like_count
		FROM answers a
		LEFT JOIN answer_likes al ON al.answer_id = a.id
		WHERE a.question_id = ?
		GROUP BY a.id, a.question_id, a.author_user_id, a.author_role, a.body, a.created_at, a.updated_at
		ORDER BY like_count DESC, a.created_at ASC, a.id ASC
		LIMIT ? OFFSET ?
	`, viewerUserID, questionID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*repository.AnswerWithLikes
	for rows.Next() {
		aw, err := r.scanAnswerWithLikes(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, aw)
	}
	return list, rows.Err()
}

func (r *MySQLAnswerRepository) CountAnswersByQuestionID(ctx context.Context, questionID int64) (int, error) {
	var total int
	err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM answers WHERE question_id = ?`, questionID).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

func (r *MySQLAnswerRepository) UpdateAnswerBody(ctx context.Context, answerID, authorUserID int64, body string) (bool, error) {
	encrypted, err := r.cipher.Encrypt(body)
	if err != nil {
		return false, fmt.Errorf("encrypt answer body: %w", err)
	}
	result, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`UPDATE answers SET body = ?, updated_at = ? WHERE id = ? AND author_user_id = ?`,
		encrypted, time.Now().Unix(), answerID, authorUserID,
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

func (r *MySQLAnswerRepository) DeleteAnswer(ctx context.Context, answerID, authorUserID int64) (bool, error) {
	result, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`DELETE FROM answers WHERE id = ? AND author_user_id = ?`,
		answerID, authorUserID,
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

func (r *MySQLAnswerRepository) LikeAnswer(ctx context.Context, answerID, userID int64) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`INSERT IGNORE INTO answer_likes (answer_id, user_id, created_at) VALUES (?, ?, ?)`,
		answerID, userID, time.Now().Unix(),
	)
	return err
}

func (r *MySQLAnswerRepository) UnlikeAnswer(ctx context.Context, answerID, userID int64) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`DELETE FROM answer_likes WHERE answer_id = ? AND user_id = ?`,
		answerID, userID,
	)
	return err
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

func (r *MySQLAnswerRepository) scanAnswerWithLikes(row answerScanner) (*repository.AnswerWithLikes, error) {
	var a model.Answer
	var createdAt, updatedAt int64
	var likeCount int
	var myLikeCount sql.NullInt64
	if err := row.Scan(&a.ID, &a.QuestionID, &a.AuthorUserID, &a.AuthorRole, &a.Body, &createdAt, &updatedAt, &likeCount, &myLikeCount); err != nil {
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

	return &repository.AnswerWithLikes{
		Answer:    &a,
		LikeCount: likeCount,
		LikedByMe: myLikeCount.Valid && myLikeCount.Int64 > 0,
	}, nil
}
