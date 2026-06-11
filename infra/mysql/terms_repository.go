package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.TermsRepository = &MySQLTermsRepository{}

type MySQLTermsRepository struct {
	DB *sql.DB
}

func NewMySQLTermsRepository(db *sql.DB) *MySQLTermsRepository {
	return &MySQLTermsRepository{DB: db}
}

func (r *MySQLTermsRepository) Save(ctx context.Context, t *model.TermsOfService) error {
	query := `
		INSERT INTO terms_of_service (version, object_key, effective_date, created_at)
		VALUES (?, ?, ?, ?)
	`
	result, err := r.DB.ExecContext(ctx, query,
		t.Version, t.ObjectKey, t.EffectiveDate.Unix(), t.CreatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert terms_of_service: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	t.ID = id
	return nil
}

func (r *MySQLTermsRepository) FindByID(ctx context.Context, id int64) (*model.TermsOfService, error) {
	query := `
		SELECT id, version, object_key, effective_date, created_at
		FROM terms_of_service
		WHERE id = ?
		LIMIT 1
	`
	row := r.DB.QueryRowContext(ctx, query, id)
	return scanTerms(row)
}

func (r *MySQLTermsRepository) FindCurrent(ctx context.Context) (*model.TermsOfService, error) {
	now := time.Now().Unix()
	query := `
		SELECT id, version, object_key, effective_date, created_at
		FROM terms_of_service
		WHERE effective_date <= ?
		ORDER BY effective_date DESC
		LIMIT 1
	`
	row := r.DB.QueryRowContext(ctx, query, now)
	return scanTerms(row)
}

func (r *MySQLTermsRepository) SaveConsent(ctx context.Context, c *model.TermsConsent) error {
	query := `
		INSERT INTO terms_consents (user_id, terms_id, consented_at)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE consented_at = VALUES(consented_at)
	`
	result, err := r.DB.ExecContext(ctx, query, c.UserID, c.TermsID, c.ConsentedAt.Unix())
	if err != nil {
		return fmt.Errorf("failed to insert terms_consent: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	if id != 0 {
		c.ID = id
	}
	return nil
}

func (r *MySQLTermsRepository) FindFuture(ctx context.Context) ([]*model.TermsOfService, error) {
	now := time.Now().Unix()
	query := `
		SELECT id, version, object_key, effective_date, created_at
		FROM terms_of_service
		WHERE effective_date > ?
		ORDER BY effective_date ASC
	`
	rows, err := r.DB.QueryContext(ctx, query, now)
	if err != nil {
		return nil, fmt.Errorf("failed to query future terms_of_service: %w", err)
	}
	defer rows.Close()
	var list []*model.TermsOfService
	for rows.Next() {
		var t model.TermsOfService
		var effectiveDateUnix, createdAtUnix int64
		if err := rows.Scan(&t.ID, &t.Version, &t.ObjectKey, &effectiveDateUnix, &createdAtUnix); err != nil {
			return nil, fmt.Errorf("failed to scan terms_of_service: %w", err)
		}
		t.EffectiveDate = time.Unix(effectiveDateUnix, 0)
		t.CreatedAt = time.Unix(createdAtUnix, 0)
		list = append(list, &t)
	}
	return list, rows.Err()
}

func (r *MySQLTermsRepository) FindAll(ctx context.Context) ([]*model.TermsOfService, error) {
	query := `
		SELECT id, version, object_key, effective_date, created_at
		FROM terms_of_service
		ORDER BY effective_date DESC
	`
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query terms_of_service: %w", err)
	}
	defer rows.Close()
	var list []*model.TermsOfService
	for rows.Next() {
		var t model.TermsOfService
		var effectiveDateUnix, createdAtUnix int64
		if err := rows.Scan(&t.ID, &t.Version, &t.ObjectKey, &effectiveDateUnix, &createdAtUnix); err != nil {
			return nil, fmt.Errorf("failed to scan terms_of_service: %w", err)
		}
		t.EffectiveDate = time.Unix(effectiveDateUnix, 0)
		t.CreatedAt = time.Unix(createdAtUnix, 0)
		list = append(list, &t)
	}
	return list, rows.Err()
}

func (r *MySQLTermsRepository) FindConsentsByTermsID(ctx context.Context, termsID int64, limit, offset int) ([]*model.TermsConsent, int, error) {
	var total int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM terms_consents WHERE terms_id = ?`, termsID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count terms_consents: %w", err)
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, user_id, terms_id, consented_at
		FROM terms_consents
		WHERE terms_id = ?
		ORDER BY consented_at DESC
		LIMIT ? OFFSET ?`, termsID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query terms_consents: %w", err)
	}
	defer rows.Close()
	var list []*model.TermsConsent
	for rows.Next() {
		var c model.TermsConsent
		var consentedAtUnix int64
		if err := rows.Scan(&c.ID, &c.UserID, &c.TermsID, &consentedAtUnix); err != nil {
			return nil, 0, fmt.Errorf("failed to scan terms_consent: %w", err)
		}
		c.ConsentedAt = time.Unix(consentedAtUnix, 0)
		list = append(list, &c)
	}
	return list, total, rows.Err()
}

func (r *MySQLTermsRepository) FindConsent(ctx context.Context, userID, termsID int64) (*model.TermsConsent, error) {
	query := `
		SELECT id, user_id, terms_id, consented_at
		FROM terms_consents
		WHERE user_id = ? AND terms_id = ?
		LIMIT 1
	`
	row := r.DB.QueryRowContext(ctx, query, userID, termsID)
	var c model.TermsConsent
	var consentedAtUnix int64
	if err := row.Scan(&c.ID, &c.UserID, &c.TermsID, &consentedAtUnix); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan terms_consent: %w", err)
	}
	c.ConsentedAt = time.Unix(consentedAtUnix, 0)
	return &c, nil
}

func scanTerms(row *sql.Row) (*model.TermsOfService, error) {
	var t model.TermsOfService
	var effectiveDateUnix, createdAtUnix int64
	if err := row.Scan(&t.ID, &t.Version, &t.ObjectKey, &effectiveDateUnix, &createdAtUnix); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan terms_of_service: %w", err)
	}
	t.EffectiveDate = time.Unix(effectiveDateUnix, 0)
	t.CreatedAt = time.Unix(createdAtUnix, 0)
	return &t, nil
}
