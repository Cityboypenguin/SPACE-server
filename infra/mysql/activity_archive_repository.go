package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

const activityArchiveLockName = "space:user_activity_archive"

var activityArchiveJST = time.FixedZone("Asia/Tokyo", 9*60*60)

const activityDateTimeFormat = "2006-01-02 15:04:05"

type MySQLActivityArchiveRepository struct {
	DB *sql.DB
}

func NewMySQLActivityArchiveRepository(db *sql.DB) *MySQLActivityArchiveRepository {
	return &MySQLActivityArchiveRepository{DB: db}
}

func (r *MySQLActivityArchiveRepository) AcquireLock(ctx context.Context) (func() error, bool, error) {
	conn, err := r.DB.Conn(ctx)
	if err != nil {
		return nil, false, err
	}
	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, 0)`, activityArchiveLockName).Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, false, err
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		_ = conn.Close()
		return func() error { return nil }, false, nil
	}

	var once sync.Once
	var releaseErr error
	release := func() error {
		once.Do(func() {
			releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var released sql.NullInt64
			if err := conn.QueryRowContext(releaseCtx, `SELECT RELEASE_LOCK(?)`, activityArchiveLockName).Scan(&released); err != nil {
				releaseErr = err
			} else if !released.Valid || released.Int64 != 1 {
				releaseErr = fmt.Errorf("activity archive lock was not held by its connection")
			}
			if err := conn.Close(); releaseErr == nil && err != nil {
				releaseErr = err
			}
		})
		return releaseErr
	}
	return release, true, nil
}

func activityDateTime(t time.Time) string {
	return t.In(activityArchiveJST).Format(activityDateTimeFormat)
}

func parseActivityDateTime(value string) (time.Time, error) {
	return time.ParseInLocation(activityDateTimeFormat, value, activityArchiveJST)
}

func (r *MySQLActivityArchiveRepository) OldestActivityHourBefore(ctx context.Context, cutoff time.Time) (*time.Time, error) {
	var oldest sql.NullString
	if err := r.DB.QueryRowContext(ctx,
		`SELECT DATE_FORMAT(MIN(activity_hour), '%Y-%m-%d %H:%i:%s') FROM user_activity_hours WHERE activity_hour < ?`, activityDateTime(cutoff),
	).Scan(&oldest); err != nil {
		return nil, err
	}
	if !oldest.Valid {
		return nil, nil
	}
	parsed, err := parseActivityDateTime(oldest.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (r *MySQLActivityArchiveRepository) StreamActivityHours(ctx context.Context, from, to time.Time, visit func(repository.ActivityHour) error) (int64, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT user_id, DATE_FORMAT(activity_hour, '%Y-%m-%d %H:%i:%s')
		FROM user_activity_hours
		WHERE activity_hour >= ? AND activity_hour < ?
		ORDER BY activity_hour ASC, user_id ASC`, activityDateTime(from), activityDateTime(to))
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count int64
	for rows.Next() {
		var row repository.ActivityHour
		var activityHour string
		if err := rows.Scan(&row.UserID, &activityHour); err != nil {
			return 0, err
		}
		row.ActivityHour, err = parseActivityDateTime(activityHour)
		if err != nil {
			return 0, err
		}
		if err := visit(row); err != nil {
			return 0, err
		}
		count++
	}
	return count, rows.Err()
}

func (r *MySQLActivityArchiveRepository) FinalizeActivityArchive(ctx context.Context, archive repository.ActivityArchive) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	to := archive.Month.AddDate(0, 1, 0)
	result, err := tx.ExecContext(ctx,
		`DELETE FROM user_activity_hours WHERE activity_hour >= ? AND activity_hour < ?`, activityDateTime(archive.Month), activityDateTime(to))
	if err != nil {
		return err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if deleted != archive.RowCount {
		return fmt.Errorf("activity archive row count changed: uploaded=%d deleted=%d", archive.RowCount, deleted)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_activity_archives
			(archive_month, object_key, row_count, sha256, archived_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		archive.Month.In(activityArchiveJST).Format("2006-01-02"), archive.ObjectKey, archive.RowCount, archive.SHA256,
		archive.ArchivedAt.Unix(), archive.ExpiresAt.Unix())
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *MySQLActivityArchiveRepository) ListExpiredActivityArchives(ctx context.Context, now time.Time) ([]repository.ActivityArchive, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT archive_month, object_key, row_count, sha256, archived_at, expires_at
		FROM user_activity_archives
		WHERE expires_at <= ?
		ORDER BY archive_month ASC`, now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []repository.ActivityArchive
	for rows.Next() {
		var item repository.ActivityArchive
		var month string
		var archivedAt, expiresAt int64
		if err := rows.Scan(&month, &item.ObjectKey, &item.RowCount, &item.SHA256, &archivedAt, &expiresAt); err != nil {
			return nil, err
		}
		item.Month, err = time.ParseInLocation("2006-01-02", month, activityArchiveJST)
		if err != nil {
			return nil, err
		}
		item.ArchivedAt = time.Unix(archivedAt, 0)
		item.ExpiresAt = time.Unix(expiresAt, 0)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *MySQLActivityArchiveRepository) DeleteActivityArchiveRecord(ctx context.Context, month time.Time) error {
	_, err := r.DB.ExecContext(ctx, `DELETE FROM user_activity_archives WHERE archive_month = ?`, month.In(activityArchiveJST).Format("2006-01-02"))
	return err
}

var _ repository.ActivityArchiveRepository = (*MySQLActivityArchiveRepository)(nil)
