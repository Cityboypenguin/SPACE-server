package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.ReportRepository = &MySQLReportRepository{}

type MySQLReportRepository struct {
	DB *sql.DB
}

func NewMySQLReportRepository(db *sql.DB) repository.ReportRepository {
	return &MySQLReportRepository{DB: db}
}

func (r *MySQLReportRepository) Save(ctx context.Context, report *model.Report) error {
	query := `
		INSERT INTO user_reports (id, reporter_id, target_type, target_id, reason, custom_reason, content, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.DB.ExecContext(ctx, query,
		report.ID,
		report.ReporterID,
		string(report.TargetType),
		report.TargetID,
		report.Reason,
		report.CustomReason,
		report.PostContent,
		string(report.Status),
		report.CreatedAt.Unix(),
		report.UpdatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert report: %w", err)
	}
	return nil
}

func scanReport(s interface {
	Scan(...any) error
}) (*model.Report, error) {
	var r model.Report
	var targetTypeStr, statusStr string
	var createdAtUnix, updatedAtUnix int64
	var content sql.NullString
	if err := s.Scan(
		&r.ID, &r.ReporterID, &targetTypeStr, &r.TargetID,
		&r.Reason, &r.CustomReason, &content, &statusStr, &createdAtUnix, &updatedAtUnix,
	); err != nil {
		return nil, err
	}
	r.TargetType = model.ReportTargetType(targetTypeStr)
	r.Status = model.ReportStatus(statusStr)
	r.CreatedAt = time.Unix(createdAtUnix, 0)
	r.UpdatedAt = time.Unix(updatedAtUnix, 0)
	if content.Valid {
		r.PostContent = &content.String
	}
	return &r, nil
}

func (r *MySQLReportRepository) FindByID(ctx context.Context, id string) (*model.Report, error) {
	query := `
		SELECT id, reporter_id, target_type, target_id, reason, custom_reason, content, status, created_at, updated_at
        FROM user_reports
        WHERE id = ?
        LIMIT 1
    `
	row := r.DB.QueryRowContext(ctx, query, id)
	report, err := scanReport(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan report: %w", err)
	}
	return report, nil
}

func (r *MySQLReportRepository) UpdateStatus(ctx context.Context, id string, status model.ReportStatus) (*model.Report, error) {
	now := time.Now()
	query := `
        UPDATE user_reports
        SET status = ?, updated_at = ?
        WHERE id = ?`

	_, err := r.DB.ExecContext(ctx, query, string(status), now.Unix(), id)
	if err != nil {
		return nil, fmt.Errorf("failed to update report status: %w", err)
	}

	return r.FindByID(ctx, id)
}

func (r *MySQLReportRepository) Search(ctx context.Context, filter *model.ReportSearchFilter, limit, offset int) ([]*model.Report, int, error) {
	countQuery := `SELECT COUNT(*) FROM user_reports WHERE 1=1`
	var countArgs []interface{}

	if filter != nil {
		if filter.Status != nil {
			countQuery += " AND status = ?"
			countArgs = append(countArgs, string(*filter.Status))
		}
		if filter.TargetType != nil {
			countQuery += " AND target_type = ?"
			countArgs = append(countArgs, string(*filter.TargetType))
		}
		if filter.ReporterID != nil && *filter.ReporterID != 0 {
			countQuery += " AND reporter_id = ?"
			countArgs = append(countArgs, *filter.ReporterID)
		}
	}

	var total int
	if err := r.DB.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count reports: %w", err)
	}

	query := `
		SELECT id, reporter_id, target_type, target_id, reason, custom_reason, content, status, created_at, updated_at
        FROM user_reports
        WHERE 1=1
    `
	var args []interface{}

	if filter != nil {
		if filter.Status != nil {
			query += " AND status = ?"
			args = append(args, string(*filter.Status))
		}
		if filter.TargetType != nil {
			query += " AND target_type = ?"
			args = append(args, string(*filter.TargetType))
		}
		if filter.ReporterID != nil && *filter.ReporterID != 0 {
			query += " AND reporter_id = ?"
			args = append(args, *filter.ReporterID)
		}
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query reports: %w", err)
	}
	defer rows.Close()

	var reports []*model.Report
	for rows.Next() {
		report, err := scanReport(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan row in search reports: %w", err)
		}
		reports = append(reports, report)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return reports, total, nil
}
