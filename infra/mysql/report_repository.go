package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/messagecrypto"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.ReportRepository = &MySQLReportRepository{}

type MySQLReportRepository struct {
	DB     *sql.DB
	cipher *messagecrypto.Cipher
}

func NewMySQLReportRepository(db *sql.DB) (repository.ReportRepository, error) {
	cipher, err := messagecrypto.New(os.Getenv("MESSAGE_ENCRYPTION_KEY"))
	if err != nil {
		return nil, err
	}
	return &MySQLReportRepository{DB: db, cipher: cipher}, nil
}

// reportColumns は user_reports を読むときの共通の列並び。scanReport と対になっている。
const reportColumns = "id, reporter_id, target_type, target_id, reason, custom_reason, content, message_snapshot, status, created_at, updated_at"

// encodeReportedMessage は通報時点のメッセージを保存用に暗号化する。
// messages.content は暗号化して保存しているので、DM の本文が通報をきっかけに
// 平文で残らないようにする。
func (r *MySQLReportRepository) encodeReportedMessage(m *model.ReportedMessage) (sql.NullString, error) {
	if m == nil {
		return sql.NullString{}, nil
	}
	body, err := json.Marshal(m)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("marshal reported message: %w", err)
	}
	encrypted, err := r.cipher.Encrypt(string(body))
	if err != nil {
		return sql.NullString{}, fmt.Errorf("encrypt reported message: %w", err)
	}
	return sql.NullString{String: encrypted, Valid: true}, nil
}

func (r *MySQLReportRepository) decodeReportedMessage(value sql.NullString) (*model.ReportedMessage, error) {
	if !value.Valid {
		return nil, nil
	}
	body, err := r.cipher.Decrypt(value.String)
	if err != nil {
		return nil, fmt.Errorf("decrypt reported message: %w", err)
	}
	var m model.ReportedMessage
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return nil, fmt.Errorf("unmarshal reported message: %w", err)
	}
	return &m, nil
}

func (r *MySQLReportRepository) Save(ctx context.Context, report *model.Report) error {
	snapshot, err := r.encodeReportedMessage(report.ReportedMessage)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO user_reports (` + reportColumns + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.DB.ExecContext(ctx, query,
		report.ID,
		report.ReporterID,
		string(report.TargetType),
		report.TargetID,
		report.Reason,
		report.CustomReason,
		report.PostContent,
		snapshot,
		string(report.Status),
		report.CreatedAt.Unix(),
		report.UpdatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert report: %w", err)
	}
	return nil
}

func (r *MySQLReportRepository) scanReport(s interface {
	Scan(...any) error
}) (*model.Report, error) {
	var report model.Report
	var targetTypeStr, statusStr string
	var createdAtUnix, updatedAtUnix int64
	var content, snapshot sql.NullString
	if err := s.Scan(
		&report.ID, &report.ReporterID, &targetTypeStr, &report.TargetID,
		&report.Reason, &report.CustomReason, &content, &snapshot, &statusStr, &createdAtUnix, &updatedAtUnix,
	); err != nil {
		return nil, err
	}
	report.TargetType = model.ReportTargetType(targetTypeStr)
	report.Status = model.ReportStatus(statusStr)
	report.CreatedAt = time.Unix(createdAtUnix, 0)
	report.UpdatedAt = time.Unix(updatedAtUnix, 0)
	if content.Valid {
		report.PostContent = &content.String
	}
	reportedMessage, err := r.decodeReportedMessage(snapshot)
	if err != nil {
		return nil, err
	}
	report.ReportedMessage = reportedMessage
	return &report, nil
}

func (r *MySQLReportRepository) FindByID(ctx context.Context, id string) (*model.Report, error) {
	query := `
		SELECT ` + reportColumns + `
        FROM user_reports
        WHERE id = ?
        LIMIT 1
    `
	row := r.DB.QueryRowContext(ctx, query, id)
	report, err := r.scanReport(row)
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

func (r *MySQLReportRepository) Search(ctx context.Context, filter *model.ReportSearchFilter, q repository.PageQuery) ([]*model.Report, int, error) {
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

	total, err := countForPage(ctx, r.DB, q, countQuery, countArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count reports: %w", err)
	}

	query := `
		SELECT ` + reportColumns + `
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

	query += " ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?"
	args = append(args, q.Limit, q.Offset)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query reports: %w", err)
	}
	defer rows.Close()

	var reports []*model.Report
	for rows.Next() {
		report, err := r.scanReport(rows)
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
