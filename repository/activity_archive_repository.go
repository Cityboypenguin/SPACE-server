package repository

import (
	"context"
	"time"
)

type ActivityHour struct {
	UserID       int64
	ActivityHour time.Time
}

type ActivityArchive struct {
	Month      time.Time
	ObjectKey  string
	RowCount   int64
	SHA256     string
	ArchivedAt time.Time
	ExpiresAt  time.Time
}

type ActivityArchiveRepository interface {
	// AcquireLock serializes the process-wide monthly job across server instances.
	// release must release the lock on the same database connection that acquired it.
	AcquireLock(ctx context.Context) (release func() error, acquired bool, err error)
	OldestActivityHourBefore(ctx context.Context, cutoff time.Time) (*time.Time, error)
	StreamActivityHours(ctx context.Context, from, to time.Time, visit func(ActivityHour) error) (int64, error)
	FinalizeActivityArchive(ctx context.Context, archive ActivityArchive) error
	ListExpiredActivityArchives(ctx context.Context, now time.Time) ([]ActivityArchive, error)
	DeleteActivityArchiveRecord(ctx context.Context, month time.Time) error
}
