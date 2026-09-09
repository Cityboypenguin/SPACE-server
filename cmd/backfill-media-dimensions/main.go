// media テーブルの width / height を、ストレージ上の実ファイルから埋め戻す一時的なツール。
//
// 062_add_dimensions_to_media より前にアップロードされた画像は寸法を持たない。
// 表示側はそれらに対してレイアウト確保ができず、ロード完了時に高さが変わる。
// このツールで一度埋めてしまえば、以降は全てのメッセージでリフローが起きなくなる。
//
// 画像はヘッダだけを読んで判定するため、オブジェクト全体はダウンロードしない。
// 何度実行しても安全（すでに埋まっている行は対象外）で、途中で止めても再開できる。
//
//	go run ./cmd/backfill-media-dimensions            # 実行
//	go run ./cmd/backfill-media-dimensions -dry-run   # 更新せず対象と判定結果だけ出す
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"time"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	azurerepo "github.com/Cityboypenguin/SPACE-server/infra/azure"
	miniorepo "github.com/Cityboypenguin/SPACE-server/infra/minio"
	"github.com/Cityboypenguin/SPACE-server/infra/mysql"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// image.DecodeConfig はヘッダのみを必要とするため、先頭だけ読めば足りる。
// 上限を設けておかないと、壊れたファイルで延々と読み続けることになる。
const headerReadLimit = 512 * 1024

// errUnsupportedFormat は「この行は何度試しても埋められない」ことを表す。
// 一時的な失敗と区別し、再実行の対象から外すために使う。
var errUnsupportedFormat = errors.New("未対応の画像形式")

type target struct {
	id         int64
	storageKey string
}

func main() {
	dryRun := flag.Bool("dry-run", false, "更新せず、対象と判定結果の表示だけ行う")
	batchSize := flag.Int("batch", 200, "1回のクエリで取得する件数")
	flag.Parse()

	database, err := mysql.New()
	if err != nil {
		log.Fatalf("データベースに接続できません: %v", err)
	}
	defer database.Close()

	storage, err := newStorage()
	if err != nil {
		log.Fatalf("ストレージに接続できません: %v", err)
	}

	ctx := context.Background()
	var scanned, updated, skipped, failed int

	// ID の昇順に前進するだけのスキャン。更新できなかった行も必ず追い越すため、
	// 失敗が続いても同じ行を読み直して止まることがない。
	var lastID int64
	for {
		rows, err := fetchBatch(ctx, database, lastID, *batchSize)
		if err != nil {
			log.Fatalf("対象の取得に失敗しました: %v", err)
		}
		if len(rows) == 0 {
			break
		}

		for _, t := range rows {
			scanned++
			lastID = t.id

			width, height, err := decodeDimensions(ctx, storage, t.storageKey)
			if err != nil {
				// 未対応の画像形式は再実行しても結果が変わらないため「除外」に数える。
				// ネットワークやストレージ側の一時的な失敗だけを「失敗」として扱い、
				// 再実行での再試行対象にする。
				if errors.Is(err, errUnsupportedFormat) {
					skipped++
					log.Printf("skip  id=%d key=%s: %v", t.id, t.storageKey, err)
					continue
				}
				failed++
				log.Printf("fail  id=%d key=%s: %v", t.id, t.storageKey, err)
				continue
			}
			if width <= 0 || height <= 0 {
				skipped++
				log.Printf("skip  id=%d key=%s: 寸法が不正 (%dx%d)", t.id, t.storageKey, width, height)
				continue
			}

			if *dryRun {
				log.Printf("dry   id=%d key=%s -> %dx%d", t.id, t.storageKey, width, height)
				updated++
				continue
			}

			if err := updateDimensions(ctx, database, t.id, width, height); err != nil {
				failed++
				log.Printf("fail  id=%d key=%s: %v", t.id, t.storageKey, err)
				continue
			}
			updated++
			log.Printf("ok    id=%d key=%s -> %dx%d", t.id, t.storageKey, width, height)
		}
	}

	log.Printf("完了: 対象 %d 件 / 更新 %d 件 / 除外 %d 件 / 失敗 %d 件", scanned, updated, skipped, failed)
	if failed > 0 {
		// 一時的な失敗が残っていることを終了コードで示す。再実行すれば失敗分だけを再試行できる。
		// 「除外」は再実行しても変わらないため、終了コードには影響させない。
		os.Exit(1)
	}
}

func newStorage() (repository.StorageRepository, error) {
	if os.Getenv("STORAGE_PROVIDER") == "azure" {
		return azurerepo.New()
	}
	return miniorepo.New()
}

// fetchBatch は寸法が未設定の画像メディアを ID 昇順で取得する。
func fetchBatch(ctx context.Context, db *sql.DB, afterID int64, limit int) ([]target, error) {
	const query = `
		SELECT id, storage_key
		FROM media
		WHERE content_type LIKE 'image/%'
		  -- SVG はベクターで固有のピクセル寸法を持たず、デコードもできない
		  AND content_type <> 'image/svg+xml'
		  AND (width IS NULL OR height IS NULL)
		  AND id > ?
		ORDER BY id ASC
		LIMIT ?
	`
	rows, err := db.QueryContext(ctx, query, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.storageKey); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

func updateDimensions(ctx context.Context, db *sql.DB, id int64, width, height int) error {
	_, err := db.ExecContext(ctx, `UPDATE media SET width = ?, height = ? WHERE id = ?`, width, height, id)
	return err
}

// decodeDimensions はオブジェクトの先頭だけを読み、画像の実寸を取り出す。
func decodeDimensions(ctx context.Context, storage repository.StorageRepository, storageKey string) (int, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	body, err := storage.GetObject(ctx, storageKey)
	if err != nil {
		return 0, 0, err
	}
	defer body.Close()

	cfg, _, err := image.DecodeConfig(io.LimitReader(body, headerReadLimit))
	if err != nil {
		if errors.Is(err, image.ErrFormat) {
			return 0, 0, fmt.Errorf("%w: %v", errUnsupportedFormat, err)
		}
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}
