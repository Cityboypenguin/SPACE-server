package db

import "testing"

// A local DB may already be at 113 after running the report-notification branch.
// Startup must recognize that version and leave existing data in place.
func TestStartupWithReportNotificationMigrationAlreadyApplied(t *testing.T) {
	database := throwawayDB(t)
	migrator := newMigrator(t, database)
	if err := migrator.Migrate(113); err != nil {
		t.Fatal(err)
	}
	mustExec(t, database, `INSERT INTO report_notification_recipients(email,created_at) VALUES('migration-test@example.com',1)`)
	for attempt := 0; attempt < 2; attempt++ {
		if err := RunMigrations(database); err != nil {
			t.Fatalf("startup at version 113: %v", err)
		}
	}
	version, dirty, err := migrator.Version()
	if err != nil || dirty || version != 113 {
		t.Fatalf("version=%d dirty=%t err=%v", version, dirty, err)
	}
	if count(t, database, "SELECT COUNT(*) FROM report_notification_recipients") != 1 {
		t.Fatal("startup modified existing report notification recipients")
	}
}
