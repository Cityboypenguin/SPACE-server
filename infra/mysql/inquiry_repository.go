package mysql

import (
	"context"
	"database/sql"
	"fmt"

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
		INSERT INTO inquiries (id, name, email, subject, content, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := r.DB.ExecContext(ctx, query,
		inquiry.ID,
		inquiry.Name,
		inquiry.Email,
		inquiry.Subject,
		inquiry.Content,
		inquiry.CreatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert inquiry: %w", err)
	}
	return nil
}
