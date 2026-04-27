package mysql

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func New() (*sql.DB, error) {
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	host := os.Getenv("DB_HOST")
	port := getenv("DB_PORT", "3306")
	dbName := os.Getenv("DB_NAME")

	// 必須の環境変数が設定されていない場合は、起動を停止させる（フェイルセーフ）
	if user == "" || password == "" || host == "" || dbName == "" {
		return nil, errors.New("required database environment variables are missing")
	}

	// 通信の暗号化設定（ローカル開発を維持するためデフォルトは "false"）
	// 本番環境では環境変数で "true" などを指定する
	tlsOption := getenv("DB_TLS", "false")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&tls=%s",
		user, password, host, port, dbName, tlsOption,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

// ポート番号など、機密ではない設定のフォールバック用として残す
func getenv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
