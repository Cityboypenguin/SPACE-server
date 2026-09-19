package activityarchive

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"errors"
	"io"
	"os"
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
func (s *privateStorageFake) DeletePrivateObject(context.Context, string) error {
	s.deleteCalls++
	return nil
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
	if storage.key != "analytics/user_activity_hours/2025/01.csv.gz" || len(storage.body) == 0 {
		t.Fatalf("unexpected upload: key=%q bytes=%d", storage.key, len(storage.body))
	}
	if repo.finalized == nil || repo.finalized.RowCount != 1 || repo.finalized.SHA256 == "" {
		t.Fatalf("archive was not finalized with verification metadata: %#v", repo.finalized)
	}
	if want := a.now().AddDate(archiveRetentionYears, 0, 0); !repo.finalized.ExpiresAt.Equal(want) {
		t.Fatalf("expires at %v, want %v", repo.finalized.ExpiresAt, want)
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
