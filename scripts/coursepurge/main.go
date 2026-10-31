// coursepurge removes historical course content only. Run on an isolated copy first.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

type object struct {
	Key       string `json:"key"`
	Container string `json:"container"`
}
type manifest struct {
	Target   string           `json:"target"`
	Counts   map[string]int64 `json:"counts"`
	Database string           `json:"database"`
	Account  string           `json:"account"`
	Objects  []object         `json:"objects"`
}

var tables = []string{"messages", "questions", "answers", "polls"}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
func scalar(q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, s string, args ...any) int64 {
	var n int64
	check(q.QueryRowContext(context.Background(), s, args...).Scan(&n))
	return n
}
func main() {
	// Never print driver errors: connection details may contain secrets.
	defer func() {
		if recover() != nil {
			fmt.Fprintln(os.Stderr, "Operation failed; keep the manifest and investigate locally. No credentials are printed.")
			os.Exit(1)
		}
	}()
	apply := flag.Bool("apply", false, "delete DB rows and Blob objects")
	stopped := flag.Bool("confirm-app-stopped", false, "all writers and background jobs are stopped")
	resume := flag.Bool("resume-blobs", false, "retry Blob deletion using existing manifest")
	path := flag.String("manifest", "course-purge.json", "durable deletion journal; keep until verified")
	flag.Parse()
	if (*apply || *resume) && !*stopped {
		panic("stop writers first")
	}
	cfg := mysql.NewConfig()
	cfg.User = os.Getenv("DB_USER")
	if cfg.User == "" {
		cfg.User = "root"
	}
	cfg.Passwd = os.Getenv("DB_PASSWORD")
	cfg.Net = "tcp"
	cfg.Addr = os.Getenv("DB_HOST") + ":" + os.Getenv("DB_PORT")
	if os.Getenv("DB_HOST") == "" || os.Getenv("DB_PORT") == "" {
		panic("DB_HOST and DB_PORT required")
	}
	cfg.DBName = os.Getenv("DB_NAME")
	if cfg.DBName == "" {
		cfg.DBName = "space"
	}
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	cfg.Timeout = 10 * time.Second
	db, err := sql.Open("mysql", cfg.FormatDSN())
	check(err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	if scalar(db, "SELECT COUNT(*) FROM schema_migrations WHERE version=102 AND dirty=0") != 1 {
		panic("expected clean version 102")
	}
	m := manifest{Objects: []object{}, Target: cfg.Addr, Counts: map[string]int64{}, Database: cfg.DBName, Account: os.Getenv("AZURE_STORAGE_ACCOUNT_NAME")}
	if *resume {
		b, e := os.ReadFile(*path)
		check(e)
		check(json.Unmarshal(b, &m))
		if m.Target != cfg.Addr || m.Database != cfg.DBName || m.Account != os.Getenv("AZURE_STORAGE_ACCOUNT_NAME") {
			panic("manifest mismatch")
		}
		deleteBlobs(db, m)
		return
	}
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	check(err)
	defer tx.Rollback()
	must := func(s string) { _, e := tx.Exec(s); check(e) }
	for _, t := range tables {
		query := "SELECT p.id FROM " + t + " p JOIN rooms r ON r.id=p.room_id WHERE r.type='course'"
		if t == "answers" {
			query = "SELECT a.id FROM answers a JOIN questions q ON q.id=a.question_id JOIN rooms r ON r.id=q.room_id WHERE r.type='course'"
		}
		must("CREATE TEMPORARY TABLE purge_" + t + " (id BIGINT PRIMARY KEY) AS " + query)
		m.Counts[t] = scalar(tx, "SELECT COUNT(*) FROM purge_"+t)
		fmt.Printf("%s: %d\n", t, m.Counts[t])
	}
	must(`CREATE TEMPORARY TABLE purge_media (id BIGINT PRIMARY KEY) AS
 SELECT media_id id FROM message_media WHERE message_id IN (SELECT id FROM purge_messages)
 UNION SELECT media_id FROM question_media WHERE question_id IN (SELECT id FROM purge_questions)
 UNION SELECT media_id FROM answer_media WHERE answer_id IN (SELECT id FROM purge_answers)`)
	// All known references, including separate media rows sharing the same storage key, are protected.
	protected := `SELECT m.id FROM media m WHERE
 EXISTS (SELECT 1 FROM post_media x WHERE x.media_id=m.id)
 OR EXISTS (SELECT 1 FROM profiles x WHERE x.avatar_media_id=m.id)
 OR EXISTS (SELECT 1 FROM communities x WHERE x.avatar_media_id=m.id)
 OR EXISTS (SELECT 1 FROM message_media x WHERE x.media_id=m.id AND x.message_id NOT IN (SELECT id FROM purge_messages))
 OR EXISTS (SELECT 1 FROM question_media x WHERE x.media_id=m.id AND x.question_id NOT IN (SELECT id FROM purge_questions))
 OR EXISTS (SELECT 1 FROM answer_media x WHERE x.media_id=m.id AND x.answer_id NOT IN (SELECT id FROM purge_answers))`
	must("CREATE TEMPORARY TABLE protected_media (id BIGINT PRIMARY KEY) AS " + protected)
	must("DELETE FROM purge_media WHERE id IN (SELECT id FROM protected_media)")
	pub := os.Getenv("AZURE_STORAGE_CONTAINER_NAME")
	if pub == "" {
		pub = "space-avatars"
	}
	priv := os.Getenv("MESSAGE_MEDIA_CONTAINER")
	if priv == "" {
		priv = "space-message-media"
	}
	must("CREATE TEMPORARY TABLE purge_media_copy AS SELECT id FROM purge_media")
	rows, err := tx.Query(`SELECT DISTINCT m.storage_key FROM media m JOIN purge_media p ON p.id=m.id
 WHERE NOT EXISTS (SELECT 1 FROM media other WHERE other.storage_key=m.storage_key AND other.id NOT IN (SELECT id FROM purge_media_copy))
 AND NOT EXISTS (SELECT 1 FROM terms_of_service t WHERE t.object_key=m.storage_key)
 AND NOT EXISTS (SELECT 1 FROM user_activity_archives a WHERE a.object_key=m.storage_key)`)
	check(err)
	for rows.Next() {
		var key string
		check(rows.Scan(&key))
		container := pub
		if strings.HasPrefix(key, "message-media/") {
			container = priv
		}
		m.Objects = append(m.Objects, object{key, container})
	}
	check(rows.Err())
	check(rows.Close())
	for _, t := range []string{"notifications", "user_reports"} {
		predicates := []string{}
		for _, entity := range tables {
			singular := strings.TrimSuffix(entity, "s")
			if entity == "polls" {
				singular = "poll"
			}
			predicates = append(predicates, "(LOWER(target_type)='"+singular+"' AND target_id IN (SELECT CAST(id AS CHAR) FROM purge_"+entity+"))")
		}
		where := strings.Join(predicates, " OR ")
		m.Counts[t] = scalar(tx, "SELECT COUNT(*) FROM "+t+" WHERE "+where)
		fmt.Printf("%s: %d\n", t, m.Counts[t])
		if *apply {
			must("DELETE FROM " + t + " WHERE " + where)
		}
	}
	m.Counts["media"] = scalar(tx, "SELECT COUNT(*) FROM purge_media")
	fmt.Printf("media rows: %d; Blob objects: %d\n", m.Counts["media"], len(m.Objects))
	if !*apply {
		fmt.Println("Dry run; no rows or objects deleted.")
		return
	}
	if m.Account == "" {
		panic("storage account required")
	}
	// Persist before COMMIT. A pre-commit crash is safe: resume refuses referenced keys.
	f, e := os.OpenFile(*path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	check(e)
	check(json.NewEncoder(f).Encode(m))
	check(f.Sync())
	check(f.Close())
	must("UPDATE questions SET best_answer_id=NULL WHERE id IN (SELECT id FROM purge_questions)")
	for _, t := range []string{"answers", "questions", "messages", "polls"} {
		must("DELETE FROM " + t + " WHERE id IN (SELECT id FROM purge_" + t + ")")
	}
	must("DELETE FROM media WHERE id IN (SELECT id FROM purge_media)")
	check(tx.Commit())
	deleteBlobs(db, m)
}
func deleteBlobs(db *sql.DB, m manifest) {
	for _, o := range m.Objects {
		if scalar(db, "SELECT (SELECT COUNT(*) FROM media WHERE storage_key=?) + (SELECT COUNT(*) FROM terms_of_service WHERE object_key=?) + (SELECT COUNT(*) FROM user_activity_archives WHERE object_key=?)", o.Key, o.Key, o.Key) != 0 {
			panic("referenced blob; refuse")
		}
		args := []string{"storage", "blob", "exists", "--auth-mode", "login", "--account-name", m.Account, "--container-name", o.Container, "--name", o.Key, "--query", "exists", "-o", "tsv", "--only-show-errors"}
		exists := func() bool {
			b, e := exec.Command("az", args...).Output()
			check(e)
			v := strings.TrimSpace(string(b))
			if v != "true" && v != "false" {
				panic("invalid blob existence result")
			}
			return v == "true"
		}
		if exists() {
			cmd := exec.Command("az", "storage", "blob", "delete", "--auth-mode", "login", "--account-name", m.Account, "--container-name", o.Container, "--name", o.Key, "--delete-snapshots", "include", "--only-show-errors", "-o", "none")
			check(cmd.Run())
		}
		if exists() {
			check(errors.New("blob still exists"))
		}
	}
	fmt.Printf("Verified %d Blob objects absent from live storage. Soft-delete retention still applies. Keep manifest.\n", len(m.Objects))
}
