package activityarchive

import (
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/csv"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/google/uuid"
)

const (
	databaseRetentionDays = 400
	archiveRetentionYears = 3
	archiveContentType    = "application/gzip"
	archiveRetryInterval  = 5 * time.Minute
	archiveObjectPrefix   = "analytics/user_activity_hours/"
	orphanGracePeriod     = 2 * time.Hour
)

var errArchiveBusy = errors.New("another instance is running the activity archive job")

var jst = time.FixedZone("Asia/Tokyo", 9*60*60)

type Archiver struct {
	repo       repository.ActivityArchiveRepository
	storage    repository.PrivateStorageRepository
	idKey      []byte
	now        func() time.Time
	runTimeout time.Duration
}

func New(repo repository.ActivityArchiveRepository, storage repository.PrivateStorageRepository, idKey string) (*Archiver, error) {
	if len(idKey) < 32 {
		return nil, errors.New("ACTIVITY_ARCHIVE_HMAC_KEY must contain at least 32 bytes")
	}
	return &Archiver{repo: repo, storage: storage, idKey: []byte(idKey), now: time.Now, runTimeout: 30 * time.Minute}, nil
}

// Run catches up immediately, retries failures and lock contention after five
// minutes, and otherwise runs at 03:00 JST on the first day of each month.
func (a *Archiver) Run(ctx context.Context) {
	for {
		failed := a.runAndLog(ctx)
		if ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(nextRunDelay(a.now(), failed))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func nextRunDelay(now time.Time, failed bool) time.Duration {
	if failed {
		return archiveRetryInterval
	}
	local := now.In(jst)
	next := time.Date(local.Year(), local.Month()+1, 1, 3, 0, 0, 0, jst)
	return next.Sub(local)
}

func (a *Archiver) runAndLog(parent context.Context) bool {
	ctx, cancel := context.WithTimeout(parent, a.runTimeout)
	defer cancel()
	err := a.RunOnce(ctx)
	if err == nil {
		return false
	}
	if parent.Err() != nil {
		return false
	}
	if errors.Is(err, errArchiveBusy) {
		logger.Log.Info().Str("component", "activity_archive").Msg("another instance is running the activity archive job; retrying")
	} else {
		logger.Log.Error().Err(err).Str("component", "activity_archive").Msg("activity archive run failed; retrying")
	}
	return true
}

func (a *Archiver) RunOnce(ctx context.Context) (runErr error) {
	release, acquired, err := a.repo.AcquireLock(ctx)
	if err != nil {
		return fmt.Errorf("acquire activity archive lock: %w", err)
	}
	if !acquired {
		return errArchiveBusy
	}
	defer func() {
		if err := release(); err != nil {
			logger.Log.Error().Err(err).Str("component", "activity_archive").Msg("failed to release activity archive lock")
		}
	}()

	now := a.now().In(jst)
	defer func() {
		cleanupCtx := ctx
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			var cancel context.CancelFunc
			cleanupCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
			defer cancel()
		}
		if cleanupCtx.Err() == nil {
			runErr = errors.Join(runErr, a.cleanupOrphans(cleanupCtx, now))
		}
	}()
	cutoff := now.AddDate(0, 0, -databaseRetentionDays)

	for {
		oldest, err := a.repo.OldestActivityHourBefore(ctx, cutoff)
		if err != nil {
			return err
		}
		if oldest == nil {
			break
		}

		oldestJST := oldest.In(jst)
		month := time.Date(oldestJST.Year(), oldestJST.Month(), 1, 0, 0, 0, 0, jst)
		monthEnd := month.AddDate(0, 1, 0)
		if monthEnd.After(cutoff) {
			break
		}
		archiveFile, rowCount, digest, err := a.writeArchiveFile(ctx, month, monthEnd)
		if err != nil {
			return err
		}
		if rowCount == 0 {
			_ = archiveFile.Close()
			_ = os.Remove(archiveFile.Name())
			return fmt.Errorf("oldest activity month %s returned no rows", month.Format("2006-01"))
		}
		fileName := archiveFile.Name()
		cleanupArchiveFile := func() {
			_ = archiveFile.Close()
			_ = os.Remove(fileName)
		}
		stat, err := archiveFile.Stat()
		if err != nil {
			cleanupArchiveFile()
			return err
		}
		objectID, err := uuid.NewRandom()
		if err != nil {
			cleanupArchiveFile()
			return fmt.Errorf("create activity archive object ID: %w", err)
		}
		key := fmt.Sprintf("%s%04d/%02d/%s.csv.gz", archiveObjectPrefix, month.Year(), month.Month(), objectID)
		if err := a.storage.PutPrivateObject(ctx, key, archiveContentType, archiveFile, stat.Size()); err != nil {
			cleanupArchiveFile()
			return fmt.Errorf("upload activity archive: %w", err)
		}
		if err := a.verifyPrivateObject(ctx, key, stat.Size(), digest); err != nil {
			cleanupArchiveFile()
			return fmt.Errorf("verify activity archive: %w", err)
		}
		if err := archiveFile.Close(); err != nil {
			_ = os.Remove(fileName)
			return err
		}
		if err := os.Remove(fileName); err != nil && !os.IsNotExist(err) {
			return err
		}

		archive := repository.ActivityArchive{
			Month: month, ObjectKey: key, RowCount: rowCount, SHA256: digest,
			ArchivedAt: now, ExpiresAt: now.AddDate(archiveRetentionYears, 0, 0),
		}
		if err := a.repo.FinalizeActivityArchive(ctx, archive); err != nil {
			return fmt.Errorf("finalize activity archive: %w", err)
		}
		logger.Log.Info().Str("component", "activity_archive").Str("month", month.Format("2006-01")).
			Int64("rows", rowCount).Str("sha256", digest).Msg("activity archive uploaded and verified")
	}

	expired, err := a.repo.ListExpiredActivityArchives(ctx, now)
	if err != nil {
		return err
	}
	for _, archive := range expired {
		if err := a.storage.DeletePrivateObject(ctx, archive.ObjectKey); err != nil {
			return fmt.Errorf("delete expired activity archive %s: %w", archive.ObjectKey, err)
		}
		if err := a.repo.DeleteActivityArchiveRecord(ctx, archive.Month); err != nil {
			return err
		}
	}
	return nil
}

func (a *Archiver) cleanupOrphans(ctx context.Context, now time.Time) error {
	objects, err := a.storage.ListPrivateObjects(ctx, archiveObjectPrefix)
	if err != nil {
		return fmt.Errorf("list activity archive objects: %w", err)
	}
	for _, object := range objects {
		if object.LastModified.IsZero() || now.Sub(object.LastModified) < orphanGracePeriod {
			continue
		}
		referenced, err := a.repo.IsActivityArchiveObjectReferenced(ctx, object.Key)
		if err != nil {
			return fmt.Errorf("check activity archive reference %s: %w", object.Key, err)
		}
		if referenced {
			continue
		}
		if err := a.storage.DeletePrivateObject(ctx, object.Key); err != nil {
			return fmt.Errorf("delete orphan activity archive %s: %w", object.Key, err)
		}
		logger.Log.Info().Str("component", "activity_archive").Str("object_key", object.Key).Msg("deleted orphan activity archive")
	}
	return nil
}

func (a *Archiver) verifyPrivateObject(ctx context.Context, key string, expectedSize int64, expectedSHA256 string) error {
	body, err := a.storage.OpenPrivateObject(ctx, key)
	if err != nil {
		return err
	}
	defer body.Close()
	digest := sha256.New()
	size, err := io.Copy(digest, body)
	if err != nil {
		return err
	}
	if size != expectedSize || digestHex(digest) != expectedSHA256 {
		return fmt.Errorf("uploaded archive does not match local file: size=%d want=%d sha256=%s want=%s",
			size, expectedSize, digestHex(digest), expectedSHA256)
	}
	return nil
}

func (a *Archiver) writeArchiveFile(ctx context.Context, from, to time.Time) (*os.File, int64, string, error) {
	file, err := os.CreateTemp("", "space-user-activity-*.csv.gz")
	if err != nil {
		return nil, 0, "", err
	}
	cleanup := func(err error) (*os.File, int64, string, error) {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, 0, "", err
	}

	digest := sha256.New()
	zw, err := gzip.NewWriterLevel(io.MultiWriter(file, digest), gzip.BestCompression)
	if err != nil {
		return cleanup(err)
	}
	zw.Header.ModTime = time.Unix(0, 0)
	w := csv.NewWriter(zw)
	if err := w.Write([]string{"user_id_hmac_sha256", "activity_hour_jst"}); err != nil {
		_ = zw.Close()
		return cleanup(err)
	}
	rowCount, err := a.repo.StreamActivityHours(ctx, from, to, func(row repository.ActivityHour) error {
		return w.Write([]string{
			a.pseudonymizeUserID(row.UserID),
			row.ActivityHour.In(jst).Format("2006-01-02 15:04:05"),
		})
	})
	if err != nil {
		_ = zw.Close()
		return cleanup(err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return cleanup(err)
	}
	if err := zw.Close(); err != nil {
		return cleanup(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return cleanup(err)
	}
	return file, rowCount, digestHex(digest), nil
}

func (a *Archiver) pseudonymizeUserID(userID int64) string {
	mac := hmac.New(sha256.New, a.idKey)
	_, _ = io.WriteString(mac, strconv.FormatInt(userID, 10))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func digestHex(digest hash.Hash) string {
	return fmt.Sprintf("%x", digest.Sum(nil))
}
