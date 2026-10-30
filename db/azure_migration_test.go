package db

import (
	"testing"
	"time"
)

func TestAzureMigration73To102(t *testing.T) {
	database := throwawayDB(t)
	migrator := newMigrator(t, database)
	if err := migrator.Migrate(73); err != nil {
		t.Fatal(err)
	}
	seedBeforeSplit(t, database)
	start := time.Now()
	if err := migrator.Up(); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	version, dirty, err := migrator.Version()
	if err != nil || version != 102 || dirty {
		t.Fatalf("version=%d dirty=%t err=%v", version, dirty, err)
	}
	if count(t, database, "SELECT COUNT(*) FROM messages WHERE id=100") != 1 || count(t, database, "SELECT COUNT(*) FROM user_accounts") != 2 {
		t.Fatal("migration lost non-course content or accounts")
	}
	t.Logf("fixture migration 73 -> 102: %s (not production-copy rehearsal)", elapsed)
}
