package activityarchive

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

const testArchiveHMACKey = "test-activity-archive-hmac-key-32-bytes"

func newTestArchiver(t *testing.T, repo repository.ActivityArchiveRepository, storage repository.PrivateStorageRepository) *Archiver {
	t.Helper()
	a, err := New(repo, storage, testArchiveHMACKey)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

type archiveRepoFake struct {
	oldest        *time.Time
	rows          []repository.ActivityHour
	finalized     *repository.ActivityArchive
	acquired      bool
	expired       []repository.ActivityArchive
	deleteErr     error
	deletes       int
	waitForCancel bool
	referenced    map[string]bool
	referenceErr  error
}

func (r *archiveRepoFake) IsActivityArchiveObjectReferenced(_ context.Context, key string) (bool, error) {
	if r.referenceErr != nil {
		return false, r.referenceErr
	}
	return r.referenced[key], nil
}

func (r *archiveRepoFake) AcquireLock(context.Context) (func() error, bool, error) {
	if !r.acquired {
		return func() error { return nil }, false, nil
	}
	return func() error { return nil }, true, nil
}

func (r *archiveRepoFake) OldestActivityHourBefore(ctx context.Context, _ time.Time) (*time.Time, error) {
	if r.waitForCancel {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return r.oldest, nil
}
func (r *archiveRepoFake) StreamActivityHours(_ context.Context, _, _ time.Time, visit func(repository.ActivityHour) error) (int64, error) {
	for _, row := range r.rows {
		if err := visit(row); err != nil {
			return 0, err
		}
	}
	return int64(len(r.rows)), nil
}
func (r *archiveRepoFake) FinalizeActivityArchive(_ context.Context, archive repository.ActivityArchive) error {
	r.finalized = &archive
	r.oldest = nil
	return nil
}
func (r *archiveRepoFake) ListExpiredActivityArchives(context.Context, time.Time) ([]repository.ActivityArchive, error) {
	return r.expired, nil
}
func (r *archiveRepoFake) DeleteActivityArchiveRecord(context.Context, time.Time) error {
	r.deletes++
	if r.deleteErr != nil {
		err := r.deleteErr
		r.deleteErr = nil
		return err
	}
	r.expired = nil
	return nil
}

type privateStorageFake struct {
	key         string
	body        []byte
	deleteCalls int
	corrupt     bool
	readError   error
	objects     []repository.PrivateObject
	listError   error
	deletedKeys []string
}

func (s *privateStorageFake) ListPrivateObjects(context.Context, string) ([]repository.PrivateObject, error) {
	return s.objects, s.listError
}

func (s *privateStorageFake) PutPrivateObject(_ context.Context, key, _ string, body io.Reader, _ int64) error {
	s.key = key
	s.body, _ = io.ReadAll(body)
	return nil
}
func (s *privateStorageFake) OpenPrivateObject(context.Context, string) (io.ReadCloser, error) {
	if s.readError != nil {
		return nil, s.readError
	}
	if s.corrupt {
		body := append([]byte(nil), s.body...)
		body[len(body)-1] ^= 1
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	return io.NopCloser(bytes.NewReader(s.body)), nil
}
func (s *privateStorageFake) DeletePrivateObject(_ context.Context, key string) error {
	s.deleteCalls++
	s.deletedKeys = append(s.deletedKeys, key)
	return nil
}

func TestCleanupOrphansKeepsReferencedAndRecentObjects(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, jst)
	repo := &archiveRepoFake{acquired: true, referenced: map[string]bool{"referenced": true}}
	storage := &privateStorageFake{objects: []repository.PrivateObject{
		{Key: "referenced", LastModified: now.Add(-24 * time.Hour)},
		{Key: "recent", LastModified: now.Add(-time.Hour)},
		{Key: "orphan", LastModified: now.Add(-24 * time.Hour)},
	}}
	a := newTestArchiver(t, repo, storage)
	a.now = func() time.Time { return now }
	if err := a.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(storage.deletedKeys) != 1 || storage.deletedKeys[0] != "orphan" {
		t.Fatalf("deleted keys = %v, want only orphan", storage.deletedKeys)
	}

	storage.deletedKeys = nil
	repo.referenceErr = errors.New("database unavailable")
	if err := a.RunOnce(context.Background()); err == nil {
		t.Fatal("reference lookup failure must stop orphan deletion")
	}
	if len(storage.deletedKeys) != 0 {
		t.Fatalf("deleted objects despite reference lookup failure: %v", storage.deletedKeys)
	}
}

func TestFailedArchiveUploadIsReclaimedAfterGracePeriod(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, jst)
	oldest := time.Date(2025, 1, 2, 10, 0, 0, 0, jst)
	repo := &archiveRepoFake{acquired: true, oldest: &oldest, rows: []repository.ActivityHour{{UserID: 7, ActivityHour: oldest}}}
	storage := &privateStorageFake{corrupt: true}
	a := newTestArchiver(t, repo, storage)
	a.now = func() time.Time { return now }
	if err := a.RunOnce(context.Background()); err == nil {
		t.Fatal("corrupt upload was finalized")
	}
	failedKey := storage.key
	repo.oldest = nil
	storage.corrupt = false
	storage.objects = []repository.PrivateObject{{Key: failedKey, LastModified: now}}
	a.now = func() time.Time { return now.Add(orphanGracePeriod + time.Minute) }
	if err := a.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(storage.deletedKeys) != 1 || storage.deletedKeys[0] != failedKey {
		t.Fatalf("failed upload was not reclaimed: %v", storage.deletedKeys)
	}
}

func TestCleanupOrphansRunsWhileArchiveKeepsFailing(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, jst)
	oldest := time.Date(2025, 1, 2, 10, 0, 0, 0, jst)
	repo := &archiveRepoFake{acquired: true, oldest: &oldest, rows: []repository.ActivityHour{{UserID: 7, ActivityHour: oldest}}}
	storage := &privateStorageFake{
		corrupt: true,
		objects: []repository.PrivateObject{{Key: "old-orphan", LastModified: now.Add(-3 * time.Hour)}},
	}
	a := newTestArchiver(t, repo, storage)
	a.now = func() time.Time { return now }
	if err := a.RunOnce(context.Background()); err == nil || !strings.Contains(err.Error(), "verify activity archive") {
		t.Fatalf("archive failure was lost: %v", err)
	}
	if len(storage.deletedKeys) != 1 || storage.deletedKeys[0] != "old-orphan" {
		t.Fatalf("orphan not reclaimed during archive failure: %v", storage.deletedKeys)
	}

	storage.deletedKeys = nil
	repo.referenceErr = errors.New("reference lookup failed")
	err := a.RunOnce(context.Background())
	if err == nil || !strings.Contains(err.Error(), "verify activity archive") || !errors.Is(err, repo.referenceErr) {
		t.Fatalf("both failures must be returned: %v", err)
	}
	if len(storage.deletedKeys) != 0 {
		t.Fatalf("deleted without reference check: %v", storage.deletedKeys)
	}
}

func TestCleanupOrphansAfterArchiveDeadline(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, jst)
	repo := &archiveRepoFake{acquired: true, waitForCancel: true}
	storage := &privateStorageFake{objects: []repository.PrivateObject{{Key: "old-orphan", LastModified: now.Add(-3 * time.Hour)}}}
	a := newTestArchiver(t, repo, storage)
	a.now = func() time.Time { return now }
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := a.RunOnce(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("archive deadline was lost: %v", err)
	}
	if len(storage.deletedKeys) != 1 || storage.deletedKeys[0] != "old-orphan" {
		t.Fatalf("orphan not reclaimed after deadline: %v", storage.deletedKeys)
	}
}

func TestNewRequiresDedicatedHMACKey(t *testing.T) {
	if _, err := New(&archiveRepoFake{}, &privateStorageFake{}, "short"); err == nil {
		t.Fatal("a short archive pseudonymization key must be rejected")
	}
}

func TestEncodeProducesDeterministicGzipCSV(t *testing.T) {
	rows := []repository.ActivityHour{{UserID: 42, ActivityHour: time.Date(2025, 8, 2, 14, 0, 0, 0, jst)}}
	first, firstDigest, err := encodedArchive(t, rows)
	if err != nil {
		t.Fatal(err)
	}
	second, secondDigest, err := encodedArchive(t, rows)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || firstDigest != secondDigest {
		t.Fatal("archive output must be deterministic for safe retries")
	}

	zr, err := gzip.NewReader(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	wantID := newTestArchiver(t, &archiveRepoFake{}, &privateStorageFake{}).pseudonymizeUserID(42)
	if len(records) != 2 || records[0][0] != "user_id_hmac_sha256" || records[1][0] != wantID || records[1][1] != "2025-08-02 14:00:00" {
		t.Fatalf("unexpected CSV records: %#v", records)
	}
}

func TestRunOnceUploadsThenFinalizesACompleteMonth(t *testing.T) {
	oldest := time.Date(2025, 1, 2, 10, 0, 0, 0, jst)
	repo := &archiveRepoFake{
		oldest:   &oldest,
		rows:     []repository.ActivityHour{{UserID: 7, ActivityHour: oldest}},
		acquired: true,
	}
	storage := &privateStorageFake{}
	a := newTestArchiver(t, repo, storage)
	a.now = func() time.Time { return time.Date(2026, 9, 19, 12, 0, 0, 0, jst) }

	if err := a.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(storage.key, "analytics/user_activity_hours/2025/01/") || !strings.HasSuffix(storage.key, ".csv.gz") || len(storage.body) == 0 {
		t.Fatalf("unexpected upload: key=%q bytes=%d", storage.key, len(storage.body))
	}
	if repo.finalized == nil || repo.finalized.RowCount != 1 || repo.finalized.SHA256 == "" {
		t.Fatalf("archive was not finalized with verification metadata: %#v", repo.finalized)
	}
	if want := a.now().AddDate(archiveRetentionYears, 0, 0); !repo.finalized.ExpiresAt.Equal(want) {
		t.Fatalf("expires at %v, want %v", repo.finalized.ExpiresAt, want)
	}
}

func TestArchiveRetriesUseDistinctObjectKeys(t *testing.T) {
	oldest := time.Date(2025, 1, 2, 10, 0, 0, 0, jst)
	storage := &privateStorageFake{}
	repo := &archiveRepoFake{oldest: &oldest, rows: []repository.ActivityHour{{UserID: 7, ActivityHour: oldest}}, acquired: true}
	a := newTestArchiver(t, repo, storage)
	a.now = func() time.Time { return time.Date(2026, 9, 19, 12, 0, 0, 0, jst) }
	if err := a.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := storage.key
	repo.oldest = &oldest
	if err := a.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if first == storage.key {
		t.Fatal("archive retries must not overwrite a previously uploaded object")
	}
}

func encodedArchive(t *testing.T, rows []repository.ActivityHour) ([]byte, string, error) {
	t.Helper()
	repo := &archiveRepoFake{rows: rows, acquired: true}
	a := newTestArchiver(t, repo, &privateStorageFake{})
	file, _, digest, err := a.writeArchiveFile(context.Background(), time.Time{}, time.Time{})
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	defer os.Remove(file.Name())
	body, err := io.ReadAll(file)
	return body, digest, err
}

func TestRunOnceSkipsWhenAnotherInstanceHoldsTheLock(t *testing.T) {
	repo := &archiveRepoFake{acquired: false}
	if err := newTestArchiver(t, repo, &privateStorageFake{}).RunOnce(context.Background()); !errors.Is(err, errArchiveBusy) {
		t.Fatalf("lock contention must schedule a retry, got %v", err)
	}
	if repo.finalized != nil {
		t.Fatal("archive must not run without the distributed lock")
	}
}

func TestRunOnceNeverFinalizesCorruptUpload(t *testing.T) {
	oldest := time.Date(2025, 1, 2, 10, 0, 0, 0, jst)
	repo := &archiveRepoFake{oldest: &oldest, rows: []repository.ActivityHour{{UserID: 7, ActivityHour: oldest}}, acquired: true}
	storage := &privateStorageFake{corrupt: true}
	a := newTestArchiver(t, repo, storage)
	a.now = func() time.Time { return time.Date(2026, 9, 19, 12, 0, 0, 0, jst) }
	if err := a.RunOnce(context.Background()); err == nil {
		t.Fatal("corrupt stored object must fail verification")
	}
	if repo.finalized != nil {
		t.Fatal("database rows must remain until the stored object is verified")
	}
}

func TestTimedOutRunIsReportedForRetry(t *testing.T) {
	repo := &archiveRepoFake{acquired: true, waitForCancel: true}
	a := newTestArchiver(t, repo, &privateStorageFake{})
	a.runTimeout = 10 * time.Millisecond
	if !a.runAndLog(context.Background()) {
		t.Fatal("a timed-out archive run must be retried")
	}
}

func TestNextRunDelayRetriesFailuresSoon(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, jst)
	if got := nextRunDelay(now, true); got != 5*time.Minute {
		t.Fatalf("failure retry = %v, want five minutes", got)
	}
	want := time.Date(2026, 10, 1, 3, 0, 0, 0, jst).Sub(now)
	if got := nextRunDelay(now, false); got != want {
		t.Fatalf("successful next run = %v, want %v", got, want)
	}
}

func TestExpiredArchiveDeletionCanResumeAfterLedgerFailure(t *testing.T) {
	month := time.Date(2022, 1, 1, 0, 0, 0, 0, jst)
	repo := &archiveRepoFake{
		acquired:  true,
		expired:   []repository.ActivityArchive{{Month: month, ObjectKey: "expired.csv.gz"}},
		deleteErr: errors.New("temporary database error"),
	}
	storage := &privateStorageFake{}
	a := newTestArchiver(t, repo, storage)

	if err := a.RunOnce(context.Background()); err == nil {
		t.Fatal("the first ledger deletion must fail")
	}
	if err := a.RunOnce(context.Background()); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if storage.deleteCalls != 2 || repo.deletes != 2 || len(repo.expired) != 0 {
		t.Fatalf("retry state: blob deletes=%d ledger deletes=%d expired=%d", storage.deleteCalls, repo.deletes, len(repo.expired))
	}
}
