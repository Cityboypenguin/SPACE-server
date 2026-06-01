package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.AnnouncementRepository = &MySQLAnnouncementRepository{}

type MySQLAnnouncementRepository struct {
	DB *sql.DB
}

func NewMySQLAnnouncementRepository(db *sql.DB) *MySQLAnnouncementRepository {
	return &MySQLAnnouncementRepository{DB: db}
}

func (r *MySQLAnnouncementRepository) Save(ctx context.Context, a *model.Announcement) error {
	query := `
		INSERT INTO announcements (title, body, admin_id, created_at)
		VALUES (?, ?, ?, ?)
	`
	result, err := r.DB.ExecContext(ctx, query, a.Title, a.Body, a.AdminID, a.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("failed to insert announcement: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	a.ID = id
	return nil
}

func (r *MySQLAnnouncementRepository) FindByID(ctx context.Context, id int64) (*model.Announcement, error) {
	query := `
		SELECT id, title, body, admin_id, created_at
		FROM announcements
		WHERE id = ?
		LIMIT 1
	`
	row := r.DB.QueryRowContext(ctx, query, id)
	var a model.Announcement
	var createdAtUnix int64
	if err := row.Scan(&a.ID, &a.Title, &a.Body, &a.AdminID, &createdAtUnix); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan announcement: %w", err)
	}
	a.CreatedAt = time.Unix(createdAtUnix, 0)
	return &a, nil
}

func (r *MySQLAnnouncementRepository) ListAll(ctx context.Context, limit int) ([]*model.Announcement, error) {
	query := `
		SELECT id, title, body, admin_id, created_at
		FROM announcements
		ORDER BY created_at DESC
		LIMIT ?
	`
	rows, err := r.DB.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query announcements: %w", err)
	}
	defer rows.Close()

	var list []*model.Announcement
	for rows.Next() {
		var a model.Announcement
		var createdAtUnix int64
		if err := rows.Scan(&a.ID, &a.Title, &a.Body, &a.AdminID, &createdAtUnix); err != nil {
			return nil, fmt.Errorf("failed to scan announcement: %w", err)
		}
		a.CreatedAt = time.Unix(createdAtUnix, 0)
		list = append(list, &a)
	}
	return list, rows.Err()
}

func (r *MySQLAnnouncementRepository) ListAllUserIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id FROM users WHERE status = 'active'`)
	if err != nil {
		return nil, fmt.Errorf("failed to query user ids: %w", err)
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
