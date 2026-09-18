package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	_ "github.com/go-sql-driver/mysql"
)

// お気に入りの二重登録は、Go 側の事前 SELECT ではなく favorites の UNIQUE 制約
// （unique_user_post）で弾いている。「同時に2回押されても1件しか入らない」ことと、
// その制約違反が repository.ErrDuplicateKey として上がってくることは、実際に
// MySQL へ並行に投げないと確かめられないので DB を使う統合テストにしてある。
//
// 常用の go test ./... で MySQL を要求したくないので、DSN が渡されたときだけ走る。
//
//	SPACE_TEST_MYSQL_DSN='root:pass@tcp(127.0.0.1:3306)/' go test ./infra/mysql/ -run Favorite
//
// スキーマ名は末尾に付けない（テストが使い捨てスキーマを自分で作る）。開発用の
// space スキーマには一切触らない。
func favoriteTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	dsn := os.Getenv("SPACE_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SPACE_TEST_MYSQL_DSN is not set; skipping the MySQL-backed favorite test")
	}

	root, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("failed to open MySQL: %v", err)
	}
	if err := root.Ping(); err != nil {
		root.Close()
		t.Fatalf("failed to reach MySQL: %v", err)
	}

	schema := fmt.Sprintf("space_favorite_test_%d", os.Getpid())
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

	// db/migrations の 004 + 021 相当から外部キーだけ落とした形。
	// 見たいのは UNIQUE 制約の効き方で、参照整合性はここの関心ではない。
	if _, err := db.Exec(`CREATE TABLE favorites (
		id BIGINT NOT NULL AUTO_INCREMENT,
		post_id BIGINT NOT NULL,
		user_id BIGINT NOT NULL,
		created_at BIGINT NOT NULL,
		PRIMARY KEY (id),
		UNIQUE KEY unique_user_post (user_id, post_id)
	)`); err != nil {
		db.Close()
		root.Close()
		t.Fatalf("failed to create the favorites table: %v", err)
	}

	return db, func() {
		db.Close()
		if _, err := root.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
			t.Errorf("failed to clean up the throwaway schema %s: %v", schema, err)
		}
		root.Close()
	}
}

func TestCreateFavorite_DuplicateSurfacesAsErrDuplicateKey(t *testing.T) {
	db, cleanup := favoriteTestDB(t)
	defer cleanup()

	repo := NewMySQLFavoriteRepository(db)
	ctx := context.Background()

	if _, err := repo.CreateFavorite(ctx, &model.Favorite{UserID: 7, PostID: 42}); err != nil {
		t.Fatalf("first CreateFavorite failed: %v", err)
	}

	_, err := repo.CreateFavorite(ctx, &model.Favorite{UserID: 7, PostID: 42})
	if err == nil {
		t.Fatal("second CreateFavorite succeeded; the UNIQUE constraint is not being enforced")
	}
	if !errors.Is(err, repository.ErrDuplicateKey) {
		t.Fatalf("error = %v, want it to wrap repository.ErrDuplicateKey", err)
	}

	// 別の投稿・別のユーザーは制約に掛からない。
	if _, err := repo.CreateFavorite(ctx, &model.Favorite{UserID: 7, PostID: 43}); err != nil {
		t.Fatalf("a different post must be insertable: %v", err)
	}
	if _, err := repo.CreateFavorite(ctx, &model.Favorite{UserID: 8, PostID: 42}); err != nil {
		t.Fatalf("a different user must be insertable: %v", err)
	}
}

// 事前 SELECT では防げなかった競合（確認と挿入の間に割り込まれる）が、UNIQUE 制約なら
// 確実に1件に収まることを確認する。
func TestCreateFavorite_ConcurrentInsertsKeepOneRow(t *testing.T) {
	db, cleanup := favoriteTestDB(t)
	defer cleanup()

	repo := NewMySQLFavoriteRepository(db)

	const attempts = 8
	var wg sync.WaitGroup
	errs := make([]error, attempts)
	start := make(chan struct{})
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = repo.CreateFavorite(context.Background(), &model.Favorite{UserID: 7, PostID: 42})
		}(i)
	}
	close(start)
	wg.Wait()

	succeeded := 0
	for i, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, repository.ErrDuplicateKey):
			// 想定どおり（先に入れた側に負けた）
		default:
			t.Fatalf("attempt %d failed with an unexpected error: %v", i, err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d concurrent inserts succeeded, want exactly 1", succeeded)
	}

	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM favorites WHERE user_id = 7 AND post_id = 42`).Scan(&rows); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("favorites rows = %d, want 1", rows)
	}
}
