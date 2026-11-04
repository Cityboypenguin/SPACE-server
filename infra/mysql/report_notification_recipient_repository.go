package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.ReportNotificationRecipientRepository = &MySQLReportNotificationRecipientRepository{}

type MySQLReportNotificationRecipientRepository struct {
	DB *sql.DB
}

func NewMySQLReportNotificationRecipientRepository(db *sql.DB) repository.ReportNotificationRecipientRepository {
	return &MySQLReportNotificationRecipientRepository{DB: db}
}

func (r *MySQLReportNotificationRecipientRepository) List(ctx context.Context) ([]*model.ReportNotificationRecipient, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id, email, created_at FROM report_notification_recipients ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("failed to query report notification recipients: %w", err)
	}
	defer rows.Close()

	recipients := []*model.ReportNotificationRecipient{}
	for rows.Next() {
		var recipient model.ReportNotificationRecipient
		var createdAtUnix int64
		if err := rows.Scan(&recipient.ID, &recipient.Email, &createdAtUnix); err != nil {
			return nil, fmt.Errorf("failed to scan report notification recipient: %w", err)
		}
		recipient.CreatedAt = time.Unix(createdAtUnix, 0)
		recipients = append(recipients, &recipient)
	}
	return recipients, rows.Err()
}

func (r *MySQLReportNotificationRecipientRepository) Create(ctx context.Context, recipient *model.ReportNotificationRecipient) error {
	result, err := r.DB.ExecContext(ctx,
		`INSERT INTO report_notification_recipients (email, created_at) VALUES (?, ?)`,
		recipient.Email, recipient.CreatedAt.Unix(),
	)
	if err != nil {
		return wrapDuplicateKey(err)
	}
	recipient.ID, err = result.LastInsertId()
	return err
}

func (r *MySQLReportNotificationRecipientRepository) Delete(ctx context.Context, id int64) (bool, error) {
	result, err := r.DB.ExecContext(ctx, `DELETE FROM report_notification_recipients WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("failed to delete report notification recipient: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}
