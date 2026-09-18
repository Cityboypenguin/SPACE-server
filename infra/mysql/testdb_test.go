package mysql

import (
	"database/sql"
	"fmt"
	"os"
	"sync/atomic"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

// ■ MySQL を使うテストの共通の土台
//
// SQL そのものが仕様になっている箇所（集計の除外条件、INSERT ... SELECT の宛先、
// バルク INSERT の結果）は、Go 側のモックでは確かめられない。そういうテストだけ
// 実物の MySQL に当てる。
//
// 常用の go test ./... で MySQL を要求したくないので、DSN が渡されたときだけ走る。
//
//	SPACE_TEST_MYSQL_DSN='root:pass@tcp(127.0.0.1:3306)/' go test ./infra/mysql/
//
// スキーマ名は末尾に付けない（テストが使い捨てスキーマを自分で作る）。開発用の
// space スキーマには一切触らない。

// schemaSeq は同一プロセス内で複数のテストが同時にスキーマを作っても
// 名前がぶつからないようにするための連番。
var schemaSeq atomic.Int64

// throwawaySchemaDB は使い捨てスキーマを作り、ddl を流した *sql.DB と後片付けを返す。
// ddl は db/migrations の DDL から、そのテストが読む列だけを抜き出して
// 外部キーを落とした形でよい（検証したいのは SQL の意味であってスキーマではない）。
func throwawaySchemaDB(t *testing.T, namePrefix string, ddl []string) (*sql.DB, func()) {
	t.Helper()

	dsn := os.Getenv("SPACE_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SPACE_TEST_MYSQL_DSN is not set; skipping the MySQL-backed test")
	}

	root, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("failed to open MySQL: %v", err)
	}
	if err := root.Ping(); err != nil {
		root.Close()
		t.Fatalf("failed to reach MySQL: %v", err)
	}

	schema := fmt.Sprintf("%s_%d_%d", namePrefix, os.Getpid(), schemaSeq.Add(1))
	if _, err := root.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
		root.Close()
		t.Fatalf("failed to drop the throwaway schema: %v", err)
	}
	if _, err := root.Exec("CREATE DATABASE " + schema); err != nil {
		root.Close()
		t.Fatalf("failed to create the throwaway schema: %v", err)
	}

	db, err := sql.Open("mysql", dsn+schema)
	if err != nil {
		root.Close()
		t.Fatalf("failed to open the throwaway schema: %v", err)
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			root.Close()
			t.Fatalf("failed to create a test table: %v", err)
		}
	}

	return db, func() {
		db.Close()
		if _, err := root.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
			t.Errorf("failed to clean up the throwaway schema %s: %v", schema, err)
		}
		root.Close()
	}
}

// ■ 往復回数の数え方
//
// 「1件ずつの INSERT/DELETE をまとめた」の効果は、結果を見ても分からない
// （結果は前と同じでなければならない）。分かるのは MySQL が受け取った文の数だけ
// なので、MySQL 自身のカウンタ（Com_insert など）を前後で読んで差を取る。
//
// SHOW SESSION STATUS は接続ごとの値なので、接続が1本に固定されていないと
// 数が合わない。singleConnDB がプールを1本に絞った *sql.DB を返す。

// statementCounts は数える対象の文種別ごとの実行回数。
type statementCounts struct {
	inserts int
	updates int
	deletes int
	selects int
}

func (c statementCounts) sub(prev statementCounts) statementCounts {
	return statementCounts{
		inserts: c.inserts - prev.inserts,
		updates: c.updates - prev.updates,
		deletes: c.deletes - prev.deletes,
		selects: c.selects - prev.selects,
	}
}

// singleConnDB はプールを1接続に固定する。SHOW SESSION STATUS が意味を持つように
// するためで、テスト以外でこれをやる理由は無い。
func singleConnDB(db *sql.DB) *sql.DB {
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db
}

func readStatementCounts(t *testing.T, db *sql.DB) statementCounts {
	t.Helper()
	// INSERT ... SELECT と JOIN 付きの DELETE は別のカウンタに入る
	// （Com_insert_select / Com_delete_multi）ので、同じ種別として足し込む。
	rows, err := db.Query(`SHOW SESSION STATUS WHERE Variable_name IN
		('Com_insert','Com_insert_select','Com_update','Com_update_multi','Com_delete','Com_delete_multi','Com_select')`)
	if err != nil {
		t.Fatalf("failed to read session status: %v", err)
	}
	defer rows.Close()

	var c statementCounts
	for rows.Next() {
		var name string
		var value int
		if err := rows.Scan(&name, &value); err != nil {
			t.Fatalf("failed to scan session status: %v", err)
		}
		switch name {
		case "Com_insert", "Com_insert_select":
			c.inserts += value
		case "Com_update", "Com_update_multi":
			c.updates += value
		case "Com_delete", "Com_delete_multi":
			c.deletes += value
		case "Com_select":
			c.selects += value
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("failed to iterate session status: %v", err)
	}
	return c
}

// countStatements は fn の中で MySQL が実行した INSERT/UPDATE/DELETE/SELECT の数を返す。
func countStatements(t *testing.T, db *sql.DB, fn func()) statementCounts {
	t.Helper()
	before := readStatementCounts(t, db)
	fn()
	return readStatementCounts(t, db).sub(before)
}
