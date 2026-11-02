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

// encodeReportedContent は通報時点のメッセージを保存用に暗号化する。
// messages.content は暗号化して保存しているので、DM の本文が通報をきっかけに
// 平文で残らないようにする。
func (r *MySQLReportRepository) encodeReportedContent(m *model.ReportedContent) (sql.NullString, error) {
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

func (r *MySQLReportRepository) decodeReportedContent(value sql.NullString) (*model.ReportedContent, error) {
	if !value.Valid {
		return nil, nil
	}
	body, err := r.cipher.Decrypt(value.String)
	if err != nil {
		return nil, fmt.Errorf("decrypt reported message: %w", err)
	}
	var m model.ReportedContent
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return nil, fmt.Errorf("unmarshal reported message: %w", err)
	}
	return &m, nil
}

func (r *MySQLReportRepository) Save(ctx context.Context, report *model.Report) error {
	snapshot, err := r.encodeReportedContent(report.ReportedContent)
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

// reportedMediaRow は通報の対象で絞った写し1件。
type reportedMediaRow struct {
	id      string
	content *model.ReportedContent
}

func (r *MySQLReportRepository) lockReportedContents(ctx context.Context, tx *sql.Tx, targetType model.ReportTargetType, targetID string) ([]reportedMediaRow, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, message_snapshot FROM user_reports
		WHERE target_type = ? AND target_id = ? AND message_snapshot IS NOT NULL FOR UPDATE`,
		string(targetType), targetID)
	if err != nil {
		return nil, fmt.Errorf("failed to query reported contents: %w", err)
	}
	defer rows.Close()
	var out []reportedMediaRow
	for rows.Next() {
		var row reportedMediaRow
		var snapshot sql.NullString
		if err := rows.Scan(&row.id, &snapshot); err != nil {
			return nil, fmt.Errorf("failed to scan reported content: %w", err)
		}
		if row.content, err = r.decodeReportedContent(snapshot); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *MySQLReportRepository) HasReportedMedia(ctx context.Context, targetType model.ReportTargetType, targetID, storageKey string) (bool, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT message_snapshot FROM user_reports
		WHERE target_type = ? AND target_id = ? AND message_snapshot IS NOT NULL`,
		string(targetType), targetID)
	if err != nil {
		return false, fmt.Errorf("failed to query reported contents: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var snapshot sql.NullString
		if err := rows.Scan(&snapshot); err != nil {
			return false, fmt.Errorf("failed to scan reported content: %w", err)
		}
		content, err := r.decodeReportedContent(snapshot)
		if err != nil {
			return false, err
		}
		for _, m := range content.Media {
			if m.StorageKey == storageKey {
				return true, nil
			}
		}
	}
	return false, rows.Err()
}

func (r *MySQLReportRepository) RepointReportedMedia(ctx context.Context, targetType model.ReportTargetType, targetID, oldKey, newKey string) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	rows, err := r.lockReportedContents(ctx, tx, targetType, targetID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		changed := false
		for i := range row.content.Media {
			if row.content.Media[i].StorageKey == oldKey {
				row.content.Media[i].StorageKey = newKey
				changed = true
			}
		}
		if !changed {
			continue
		}
		snapshot, err := r.encodeReportedContent(row.content)
		if err != nil {
			return err
		}
		// 通報の扱いが変わったわけではないので updated_at は動かさない。
		if _, err := tx.ExecContext(ctx, `UPDATE user_reports SET message_snapshot = ? WHERE id = ?`, snapshot, row.id); err != nil {
			return fmt.Errorf("failed to update reported content: %w", err)
		}
	}
	return tx.Commit()
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
	reportedContent, err := r.decodeReportedContent(snapshot)
	if err != nil {
		return nil, err
	}
	report.ReportedContent = reportedContent
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
