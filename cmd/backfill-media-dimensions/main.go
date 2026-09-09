// media テーブルの width / height を、ストレージ上の実ファイルから埋め戻すツール。
//
// 062_add_dimensions_to_media より前にアップロードされた画像は寸法を持たない。
// 表示側はそれらに対してレイアウト確保ができず、ロード完了時に高さが変わる。
// このツールで一度埋めてしまえば、以降は全てのメッセージでリフローが起きなくなる。
//
// 寸法は通常クライアントの自己申告で保存される。-verify は保存済みの値を実ファイルと
// 突き合わせ、食い違っていれば実測値で上書きする。誤った申告は表示比率が恒久的に
// ずれたままになるため、その受け皿として使う。
//
// 画像はヘッダだけを読んで判定するため、オブジェクト全体はダウンロードしない。
// 何度実行しても安全（すでに正しく埋まっている行は変更しない）で、途中で止めても再開できる。
//
//	go run ./cmd/backfill-media-dimensions                  # 未設定の行を埋める
//	go run ./cmd/backfill-media-dimensions -dry-run         # 更新せず対象と判定結果だけ出す
//	go run ./cmd/backfill-media-dimensions -verify          # 設定済みの行も実ファイルと突き合わせる
//	go run ./cmd/backfill-media-dimensions -concurrency 16  # 並行数を上げる
package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"
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
	width      *int
	height     *int
}

// counters は並行に更新されるため atomic で扱う。
type counters struct {
	scanned   atomic.Int64
	updated   atomic.Int64
	unchanged atomic.Int64
	skipped   atomic.Int64
	failed    atomic.Int64
}

func main() {
	dryRun := flag.Bool("dry-run", false, "更新せず、対象と判定結果の表示だけ行う")
	verify := flag.Bool("verify", false, "設定済みの行も実ファイルと突き合わせ、食い違っていれば直す")
	batchSize := flag.Int("batch", 200, "1回のクエリで取得する件数")
	concurrency := flag.Int("concurrency", 8, "同時にストレージへ問い合わせる数")
	flag.Parse()

	if *batchSize < 1 || *concurrency < 1 {
		log.Fatal("-batch と -concurrency は 1 以上を指定してください")
	}

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
	var c counters
	started := time.Now()

	// ID 昇順に前進するだけのスキャン。更新できなかった行も必ず追い越すため、
	// 失敗が続いても同じ行を読み直して止まることがない。
	var lastID int64
	for {
		rows, err := fetchBatch(ctx, database, lastID, *batchSize, *verify)
		if err != nil {
			log.Fatalf("対象の取得に失敗しました: %v", err)
		}
		if len(rows) == 0 {
			break
		}
		lastID = rows[len(rows)-1].id

		processBatch(ctx, database, storage, rows, *concurrency, *dryRun, &c)
	}

	log.Printf("完了: 対象 %d 件 / 更新 %d 件 / 変更なし %d 件 / 除外 %d 件 / 失敗 %d 件 (%s)",
		c.scanned.Load(), c.updated.Load(), c.unchanged.Load(), c.skipped.Load(), c.failed.Load(),
		time.Since(started).Round(time.Millisecond))

	if c.failed.Load() > 0 {
		// 一時的な失敗が残っていることを終了コードで示す。再実行すれば失敗分だけを再試行できる。
		// 「除外」は再実行しても変わらないため、終了コードには影響させない。
		os.Exit(1)
	}
}

// processBatch は1バッチ分をワーカーで並行処理する。ストレージ待ちが大半のため、
// 並行数を上げると全体時間がほぼ線形に縮む。
func processBatch(
	ctx context.Context,
	db *sql.DB,
	storage repository.StorageRepository,
	rows []target,
	concurrency int,
	dryRun bool,
	c *counters,
) {
	queue := make(chan target)
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range queue {
				processOne(ctx, db, storage, t, dryRun, c)
			}
		}()
	}

	for _, t := range rows {
		queue <- t
	}
	close(queue)
	wg.Wait()
}

func processOne(
	ctx context.Context,
	db *sql.DB,
	storage repository.StorageRepository,
	t target,
	dryRun bool,
	c *counters,
) {
	c.scanned.Add(1)

	width, height, err := decodeDimensions(ctx, storage, t.storageKey)
	if err != nil {
		// 未対応の画像形式は再実行しても結果が変わらないため「除外」に数える。
		// ネットワークやストレージ側の一時的な失敗だけを「失敗」として扱い、
		// 再実行での再試行対象にする。
		if errors.Is(err, errUnsupportedFormat) {
			c.skipped.Add(1)
			log.Printf("skip  id=%d key=%s: %v", t.id, t.storageKey, err)
			return
		}
		c.failed.Add(1)
		log.Printf("fail  id=%d key=%s: %v", t.id, t.storageKey, err)
		return
	}
	if width <= 0 || height <= 0 {
		c.skipped.Add(1)
		log.Printf("skip  id=%d key=%s: 寸法が不正 (%dx%d)", t.id, t.storageKey, width, height)
		return
	}

	if equalsStored(t, width, height) {
		c.unchanged.Add(1)
		return
	}

	// 保存済みの値と食い違う場合は、クライアントの申告が誤っていたことを意味する。
	// 表示比率がずれたままになるため、目立つように記録する。
	if t.width != nil && t.height != nil {
		log.Printf("diff  id=%d key=%s: 保存値 %dx%d -> 実測 %dx%d",
			t.id, t.storageKey, *t.width, *t.height, width, height)
	}

	if dryRun {
		c.updated.Add(1)
		log.Printf("dry   id=%d key=%s -> %dx%d", t.id, t.storageKey, width, height)
		return
	}

	if err := updateDimensions(ctx, db, t.id, width, height); err != nil {
		c.failed.Add(1)
		log.Printf("fail  id=%d key=%s: %v", t.id, t.storageKey, err)
		return
	}
	c.updated.Add(1)
	log.Printf("ok    id=%d key=%s -> %dx%d", t.id, t.storageKey, width, height)
}

func equalsStored(t target, width, height int) bool {
	return t.width != nil && t.height != nil && *t.width == width && *t.height == height
}

func newStorage() (repository.StorageRepository, error) {
	if os.Getenv("STORAGE_PROVIDER") == "azure" {
		return azurerepo.New()
	}
	return miniorepo.New()
}

// fetchBatch は対象の画像メディアを ID 昇順で取得する。
// verify が false のときは寸法が未設定の行だけを対象にする。
func fetchBatch(ctx context.Context, db *sql.DB, afterID int64, limit int, verify bool) ([]target, error) {
	// SVG はベクターで固有のピクセル寸法を持たず、デコードもできないため常に除外する。
	const baseQuery = `
		SELECT id, storage_key, width, height
		FROM media
		WHERE content_type LIKE 'image/%%'
		  AND content_type <> 'image/svg+xml'
		  %s
		  AND id > ?
		ORDER BY id ASC
		LIMIT ?
	`
	condition := "AND (width IS NULL OR height IS NULL)"
	if verify {
		condition = ""
	}

	rows, err := db.QueryContext(ctx, fmt.Sprintf(baseQuery, condition), afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.storageKey, &t.width, &t.height); err != nil {
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

// decodeDimensions はオブジェクトの先頭だけを読み、ブラウザが表示するのと同じ向きの
// 実寸を取り出す。EXIF の回転指定はブラウザが既定で適用するため、ここでも反映させる。
func decodeDimensions(ctx context.Context, storage repository.StorageRepository, storageKey string) (int, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	body, err := storage.GetObject(ctx, storageKey)
	if err != nil {
		return 0, 0, err
	}
	defer body.Close()

	// 向きの判定と寸法の取得で同じバイト列を二度読むため、先頭をメモリに載せる。
	// 読み切れなくても DecodeConfig はヘッダさえあれば成功する。
	header, err := io.ReadAll(io.LimitReader(body, headerReadLimit))
	if err != nil {
		return 0, 0, err
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(header))
	if err != nil {
		if errors.Is(err, image.ErrFormat) {
			return 0, 0, fmt.Errorf("%w: %v", errUnsupportedFormat, err)
		}
		return 0, 0, err
	}

	width, height := cfg.Width, cfg.Height
	if exifOrientationSwapsAxes(jpegExifOrientation(header)) {
		width, height = height, width
	}
	return width, height, nil
}
