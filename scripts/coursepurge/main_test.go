package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Real MySQL is essential here: temporary-table rules and cascades differ from mocks.
// Use an isolated server only. No production DSN, Azure CLI or credentials are used.
func TestPurgeIsolationAndBlobResume(t *testing.T) {
	dsn := os.Getenv("SPACE_TEST_MYSQL_DSN")
	binary := os.Getenv("COURSE_PURGE_BIN")
	if dsn == "" || binary == "" {
		t.Skip("isolated MySQL and COURSE_PURGE_BIN required")
	}
	cfg, e := mysql.ParseDSN(dsn)
	if e != nil {
		t.Fatal(e)
	}
	root, e := sql.Open("mysql", dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer root.Close()
	name := fmt.Sprintf("space_purge_test_%d", time.Now().UnixNano())
	if _, e = root.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4"); e != nil {
		t.Fatal(e)
	}
	defer root.Exec("DROP DATABASE " + name)
	cfg.DBName = name
	db, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE schema_migrations(version BIGINT,dirty BOOLEAN)`, `INSERT INTO schema_migrations VALUES(102,0)`,
		`CREATE TABLE rooms(id BIGINT PRIMARY KEY,type VARCHAR(32))`, `INSERT INTO rooms VALUES(1,'course'),(2,'dm')`,
		`CREATE TABLE messages(id BIGINT PRIMARY KEY,room_id BIGINT)`, `INSERT INTO messages VALUES(1,1),(2,2)`,
		`CREATE TABLE questions(id BIGINT PRIMARY KEY,room_id BIGINT,best_answer_id BIGINT NULL)`, `INSERT INTO questions VALUES(1,1,1),(2,2,NULL)`,
		`CREATE TABLE answers(id BIGINT PRIMARY KEY,question_id BIGINT,FOREIGN KEY(question_id) REFERENCES questions(id) ON DELETE CASCADE)`, `INSERT INTO answers VALUES(1,1),(2,2)`,
		`CREATE TABLE polls(id BIGINT PRIMARY KEY,room_id BIGINT)`, `INSERT INTO polls VALUES(1,1),(2,2)`,
		`CREATE TABLE media(id BIGINT PRIMARY KEY,storage_key VARCHAR(255))`,
		`INSERT INTO media VALUES(1,'media/shared'),(2,'media/only-course'),(3,'media/duplicate-key'),(4,'media/duplicate-key'),(5,'media/avatar'),(6,'media/terms')`,
		`CREATE TABLE message_media(message_id BIGINT,media_id BIGINT,FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE,FOREIGN KEY(media_id) REFERENCES media(id) ON DELETE CASCADE)`,
		`INSERT INTO message_media VALUES(1,1),(2,1),(1,2),(1,3),(1,5),(1,6)`,
		`CREATE TABLE question_media(question_id BIGINT,media_id BIGINT,FOREIGN KEY(question_id) REFERENCES questions(id) ON DELETE CASCADE)`,
		`CREATE TABLE answer_media(answer_id BIGINT,media_id BIGINT,FOREIGN KEY(answer_id) REFERENCES answers(id) ON DELETE CASCADE)`,
		`CREATE TABLE post_media(post_id BIGINT,media_id BIGINT)`,
		`CREATE TABLE profiles(avatar_media_id BIGINT)`, `INSERT INTO profiles VALUES(5)`,
		`CREATE TABLE communities(avatar_media_id BIGINT)`,
		`CREATE TABLE terms_of_service(object_key VARCHAR(255))`, `INSERT INTO terms_of_service VALUES('media/terms')`,
		`CREATE TABLE user_activity_archives(object_key VARCHAR(512))`,
		`CREATE TABLE notifications(target_type VARCHAR(64),target_id BIGINT)`, `INSERT INTO notifications VALUES('message',1),('post',1),('message',2)`,
		`CREATE TABLE user_reports(target_type VARCHAR(64),target_id VARCHAR(255))`, `INSERT INTO user_reports VALUES('QUESTION','1'),('POST','1')`,
	}
	for _, s := range statements {
		if _, e = db.Exec(s); e != nil {
			t.Fatal(e)
		}
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "az")
	present := filepath.Join(dir, "present")
	fail := filepath.Join(dir, "fail")
	journal := filepath.Join(dir, "journal.json")
	stub := `#!/bin/sh
case "$3" in
exists) if test -f "$FAKE_PRESENT"; then echo true; else echo false; fi;;
delete) if test -f "$FAKE_FAIL"; then exit 1; fi; rm -f "$FAKE_PRESENT";;
*) exit 2;;
esac
`
	if e = os.WriteFile(fake, []byte(stub), 0700); e != nil {
		t.Fatal(e)
	}
	host, port, _ := strings.Cut(cfg.Addr, ":")
	env := append(os.Environ(), "DB_HOST="+host, "DB_PORT="+port, "DB_USER="+cfg.User, "DB_PASSWORD="+cfg.Passwd, "DB_NAME="+name, "AZURE_STORAGE_ACCOUNT_NAME=test-account", "PATH="+dir+":"+os.Getenv("PATH"), "FAKE_PRESENT="+present, "FAKE_FAIL="+fail)
	run := func(args ...string) (string, error) {
		c := exec.Command(binary, args...)
		c.Env = env
		b, e := c.CombinedOutput()
		return string(b), e
	}
	output, e := run()
	if e != nil || !strings.Contains(output, "Dry run") {
		t.Fatalf("dry run: %s %v", output, e)
	}
	count := func(table string) int {
		var n int
		if e := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	if count("messages") != 2 {
		t.Fatal("dry run mutated data")
	}
	if _, e := os.Stat(journal); !os.IsNotExist(e) {
		t.Fatal("dry run wrote journal")
	}
	os.WriteFile(present, []byte("x"), 0600)
	os.WriteFile(fail, []byte("x"), 0600)
	if _, e = run("--apply", "--manifest", journal); e == nil {
		t.Fatal("missing app-stopped flag accepted")
	}
	if _, e = run("--apply", "--confirm-app-stopped", "--manifest", journal); e == nil {
		t.Fatal("expected Blob failure")
	}
	for _, table := range tables {
		if count(table) != 1 {
			t.Fatalf("wrong remaining %s", table)
		}
	}
	if count("media") != 3 || count("message_media") != 1 || count("notifications") != 2 || count("user_reports") != 1 {
		t.Fatal("reference protection or cleanup failed")
	}
	b, e := os.ReadFile(journal)
	if e != nil || !strings.Contains(string(b), "media/only-course") || strings.Contains(string(b), "media/duplicate-key") || strings.Contains(string(b), "media/terms") {
		t.Fatalf("unsafe manifest: %s %v", b, e)
	}
	var saved manifest
	if err := json.Unmarshal(b, &saved); err != nil || saved.Counts["messages"] != 1 || saved.Counts["media"] != 3 || saved.Target != cfg.Addr {
		t.Fatal("missing count or target metadata")
	}
	os.Remove(fail)
	output, e = run("--resume-blobs", "--confirm-app-stopped", "--manifest", journal)
	if e != nil || !strings.Contains(output, "Verified 1") {
		t.Fatalf("resume: %s %v", output, e)
	}
	if _, e = os.Stat(present); !os.IsNotExist(e) {
		t.Fatal("blob not deleted")
	}
	if _, e = run("--resume-blobs", "--confirm-app-stopped", "--manifest", journal); e != nil {
		t.Fatal("resume not idempotent")
	}
}
