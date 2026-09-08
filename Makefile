.PHONY: test test-race test-integration test-integration-up test-integration-down test-load

# DBを必要としない単体テストのみ実行（usecase層・model層・graph resolver層など）。
test:
	go test ./...

# データ競合検出付きで単体テストを実行。
test-race:
	go test -race ./...

# compose.test.yaml の MySQL を起動する（初回はヘルスチェック通過まで待つ）。
test-integration-up:
	docker compose -f compose.test.yaml up -d --wait

test-integration-down:
	docker compose -f compose.test.yaml down -v

# 実MySQLに接続する統合テスト（`integration` ビルドタグ付きファイルのみ）を実行する。
# 事前に test-integration-up でDBを起動しておくこと。
test-integration:
	DB_HOST=127.0.0.1 DB_PORT=3307 DB_USER=root DB_PASSWORD=test DB_NAME=space_test \
	MESSAGE_ENCRYPTION_KEY=OaI4511ozclMgQhEkD7NBgWeQmgsyjRp2jI2o+LRN3g= \
	go test -tags=integration ./...

# k6による負荷テスト（要: サーバー起動、BASE_URL等は load/README.md 参照）。
test-load:
	k6 run load/chat-history-load.js
	k6 run load/chat-realtime-load.js
