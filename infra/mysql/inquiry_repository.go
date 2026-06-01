package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.InquiryRepository = &MySQLInquiryRepository{}

type MySQLInquiryRepository struct {
	DB *sql.DB
}

func NewMySQLInquiryRepository(db *sql.DB) *MySQLInquiryRepository {
	return &MySQLInquiryRepository{DB: db}
}

func (r *MySQLInquiryRepository) Save(ctx context.Context, inquiry *model.Inquiry) error {
	query := `
		INSERT INTO inquiries (id, name, email, subject, content, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.DB.ExecContext(ctx, query,
		inquiry.ID,
		inquiry.Name,
		inquiry.Email,
		inquiry.Subject,
		inquiry.Content,
		string(inquiry.Status),
		inquiry.CreatedAt.Unix(),
		inquiry.UpdatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert inquiry: %w", err)
	}
	return nil
}

func (r *MySQLInquiryRepository) FindAll(ctx context.Context, status *model.InquiryStatus) ([]*model.Inquiry, error) {
	query := `
		SELECT id, name, email, subject, content, status, created_at, updated_at
		FROM inquiries
		WHERE 1=1
	`
	var args []interface{}
	if status != nil {
		query += " AND status = ?"
		args = append(args, string(*status))
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query inquiries: %w", err)
	}
	defer rows.Close()

	var inquiries []*model.Inquiry
	for rows.Next() {
		inq, err := scanInquiry(rows)
		if err != nil {
			return nil, err
		}
		inquiries = append(inquiries, inq)
	}
	return inquiries, rows.Err()
}

func (r *MySQLInquiryRepository) FindByID(ctx context.Context, id string) (*model.Inquiry, error) {
	query := `
		SELECT id, name, email, subject, content, status, created_at, updated_at
		FROM inquiries
		WHERE id = ?
		LIMIT 1
	`
	row := r.DB.QueryRowContext(ctx, query, id)
	inq, err := scanInquiryRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return inq, err
}

func (r *MySQLInquiryRepository) UpdateStatus(ctx context.Context, id string, status model.InquiryStatus) (*model.Inquiry, error) {
	now := time.Now()
	query := `
		UPDATE inquiries
		SET status = ?, updated_at = ?
		WHERE id = ?
	`
	result, err := r.DB.ExecContext(ctx, query, string(status), now.Unix(), id)
	if err != nil {
		return nil, fmt.Errorf("failed to update inquiry status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, fmt.Errorf("inquiry not found")
	}
	return r.FindByID(ctx, id)
}

type inquiryScanner interface {
	Scan(dest ...interface{}) error
}

func scanInquiry(s inquiryScanner) (*model.Inquiry, error) {
	var inq model.Inquiry
	var statusStr string
	var createdAtUnix, updatedAtUnix int64

	if err := s.Scan(
		&inq.ID,
		&inq.Name,
		&inq.Email,
		&inq.Subject,
		&inq.Content,
		&statusStr,
		&createdAtUnix,
		&updatedAtUnix,
	); err != nil {
		return nil, fmt.Errorf("failed to scan inquiry: %w", err)
	}

	inq.Status = model.InquiryStatus(statusStr)
	inq.CreatedAt = time.Unix(createdAtUnix, 0)
	inq.UpdatedAt = time.Unix(updatedAtUnix, 0)
	return &inq, nil
}

func scanInquiryRow(row *sql.Row) (*model.Inquiry, error) {
	var inq model.Inquiry
	var statusStr string
	var createdAtUnix, updatedAtUnix int64

	if err := row.Scan(
		&inq.ID,
		&inq.Name,
		&inq.Email,
		&inq.Subject,
		&inq.Content,
		&statusStr,
		&createdAtUnix,
		&updatedAtUnix,
	); err != nil {
		return nil, err
	}

	inq.Status = model.InquiryStatus(statusStr)
	inq.CreatedAt = time.Unix(createdAtUnix, 0)
	inq.UpdatedAt = time.Unix(updatedAtUnix, 0)
	return &inq, nil
}
