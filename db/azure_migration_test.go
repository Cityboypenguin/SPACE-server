package db

import (
	"testing"
	"time"
)

func TestAzureMigration73ToLatest(t *testing.T) {
	latest, err := LatestVersion()
	if err != nil {
		t.Fatal(err)
	}
	database := throwawayDB(t)
	migrator := newMigrator(t, database)
	if err := migrator.Migrate(73); err != nil {
		t.Fatal(err)
	}
	seedBeforeSplit(t, database)
	start := time.Now()
	// The cutover starts from the AWS v73 dump and ends wherever this commit's app migrates to.
	if err := migrator.Up(); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	version, dirty, err := migrator.Version()
	if err != nil || version != latest || dirty {
		t.Fatalf("version=%d dirty=%t latest=%d err=%v", version, dirty, latest, err)
	}
	if count(t, database, "SELECT COUNT(*) FROM messages WHERE id=100") != 1 || count(t, database, "SELECT COUNT(*) FROM user_accounts") != 2 {
		t.Fatal("migration lost non-course content or accounts")
	}
	t.Logf("fixture migration 73 -> %d: %s (not production-copy rehearsal)", latest, elapsed)
}
