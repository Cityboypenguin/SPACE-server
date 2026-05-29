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
        INSERT INTO user_reports (id, reporter_id, target_type, target_id, reason, custom_reason, status, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `

    _, err := r.DB.ExecContext(ctx, query,
        report.ID,
        report.ReporterID,
        string(report.TargetType),
        report.TargetID,
        report.Reason,
        report.CustomReason,
        string(report.Status),
        report.CreatedAt.Unix(),
        report.UpdatedAt.Unix(),
    )
    if err != nil {
        return fmt.Errorf("failed to insert report: %w", err)
    }
    return nil
}

func (r *MySQLReportRepository) FindByID(ctx context.Context, id string) (*model.Report, error) {
    query := `
        SELECT id, reporter_id, target_type, target_id, reason, custom_reason, status, created_at, updated_at
        FROM user_reports
        WHERE id = ?
        LIMIT 1
    `

    row := r.DB.QueryRowContext(ctx, query, id)

    var report model.Report
    var targetTypeStr, statusStr string
    var createdAtUnix, updatedAtUnix int64

    if err := row.Scan(
        &report.ID,
        &report.ReporterID,
        &targetTypeStr,
        &report.TargetID,
        &report.Reason,
        &report.CustomReason,
        &statusStr,
        &createdAtUnix,
        &updatedAtUnix,
    ); err != nil {
        if err == sql.ErrNoRows {
            return nil, nil
        }
        return nil, fmt.Errorf("failed to scan report: %w", err)
    }

    report.TargetType = model.ReportTargetType(targetTypeStr)
    report.Status = model.ReportStatus(statusStr)
    report.CreatedAt = time.Unix(createdAtUnix, 0)
    report.UpdatedAt = time.Unix(updatedAtUnix, 0)

    return &report, nil
}

func (r *MySQLReportRepository) UpdateStatus(ctx context.Context, id string, status model.ReportStatus) (*model.Report, error) {
    now := time.Now()
    query := `
        UPDATE user_reports
        SET status = ?, updated_at = ?
        WHERE id = ?
    `

    _, err := r.DB.ExecContext(ctx, query, string(status), now.Unix(), id)
    if err != nil {
        return nil, fmt.Errorf("failed to update report status: %w", err)
    }

    return r.FindByID(ctx, id)
}

func (r *MySQLReportRepository) Search(ctx context.Context, filter *model.ReportSearchFilter) ([]*model.Report, error) {
    query := `
        SELECT id, reporter_id, target_type, target_id, reason, custom_reason, status, created_at, updated_at
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

    query += " ORDER BY created_at DESC"

    rows, err := r.DB.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, fmt.Errorf("failed to query reports: %w", err)
    }
    defer rows.Close()

    var reports []*model.Report
    for rows.Next() {
        var report model.Report
        var targetTypeStr, statusStr string
        var createdAtUnix, updatedAtUnix int64

        if err := rows.Scan(
            &report.ID,
            &report.ReporterID,
            &targetTypeStr,
            &report.TargetID,
            &report.Reason,
            &report.CustomReason,
            &statusStr,
            &createdAtUnix,
            &updatedAtUnix,
        ); err != nil {
            return nil, fmt.Errorf("failed to scan row in search reports: %w", err)
        }

        report.TargetType = model.ReportTargetType(targetTypeStr)
        report.Status = model.ReportStatus(statusStr)
        report.CreatedAt = time.Unix(createdAtUnix, 0)
        report.UpdatedAt = time.Unix(updatedAtUnix, 0)

        reports = append(reports, &report)
    }

    if err := rows.Err(); err != nil {
        return nil, err
    }

    return reports, nil
}