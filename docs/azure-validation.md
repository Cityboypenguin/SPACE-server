# Azure 移行の検証記録

2026-10-30（日本時間）。作業ブランチ `feature/azure-vm-go-live`。
mainへのマージ/push、本番デプロイ、AWS/DNS変更、本番データ削除、VMの秘密情報変更は未実施。
既存 Azure ブランチの実装を作業ブランチへ取り込んで検証した。

## 実施済み

- `go test ./...`：成功。localhostで一時ポートを使用するテストのためsandbox外で実行。
  ローカルでは `SPACE_TEST_MYSQL_DSN` 未指定のMySQL統合テストはskip。
- Azure VMで既存MySQLとは別の `mysql:8.0` 一時コンテナーを立て、
  loopbackの13307番だけに公開してテストバイナリを実行。コンテナーと作業ファイルは終了後に削除。
  本番データ・既存コンテナー・VMの `.env` は変更していない。
- `TestAzureMigration73To102`：テストデータで clean 73→102 成功。
  マイグレーション部分 **3.590902336秒**。日本語のアカウントと既存DMが移行された。
  これは本番DBコピーの所要時間ではない。
- `TestAccountSplitMigration`：アカウント分割、外部キー、会話の保持、down方向の検証が成功。
- `TestPurgeIsolationAndBlobResume`：MySQLでドライラン不変、実行フラグ必須、
  授業投稿の削除、DM保持、通知・通報の型を含めた削除判定、共有media・重複キー・規約キー保護が成功。
  Blob CLIはスタブを使用し、DBコミット後のBlob削除失敗、記録からの再開、再実行の冪等性を検証。
  実Blobの削除は行っていない。
- Redis引継ぎ：一時Redisで失効キーだけの移行、ドライラン不変、期限切れ除外、
  絶対期限維持、宛先の長い失効期限保持、メンテナンスとOTPの除外を検証。
- Pythonテスト：アプリタグの自動切戻し、`.env` の他の値と権限保持、
  画像再投入防止、コピー後のバイト列検証、件数の差分照合を検証。
- shell構文、workflowとcomposeのYAML構文、`git diff --check` を確認。
- VMの現行 `/etc/nginx/conf.d/space.conf` を読取りで確認し、443とcertbot設定を反映。
  nginx設定の配置・reload・本番用リダイレクトの有効化はしていない。

## 未実施・本番切替前に必要

本番v73ダンプの承認済み取得元がこの作業では未指定のため、本番DBコピーでのリハーサルは未実施。
コードとテストデータでの成功を本番リハーサルの成功とは扱わない。

| 項目 | 状態 |
| --- | --- |
| 最新本番コピー73→108の成否・所要時間 | 未実施 |
| 本番参考値60/3/4/3との照合・削除 | 未実施 |
| 公開Blobの削除と匿名URL404、30日保持後の状態 | 未実施 |
| private S3/Blobアーカイブの実件数・内容照合 | 未実施。コード上は移行必要として手順を用意 |
| ACRへのpush・実VMのアプリ入替・切戻し | 未実施。Secrets登録と人間の実行が必要 |
| 本番nginx `nginx -t`、reload、HTTP転送/HSTS | 未実施 |
| ログイン・投稿・画像・アップロード | 実環境リハーサル未実施 |
| DM本文/添付の復号・新規送受信 | 実環境リハーサル未実施 |
| SSE・WebSocket接続と再接続 | 実環境リハーサル未実施 |
| 外部監視の登録・通知確認 | 提案のみ。人間による設定が必要 |

GitHub上のCIは `.github/workflows/azure-validation.yml` を追加済み。pushしていないため未実行。
本番への切替可否は、上記未実施項目の結果を埋めてから判断する。


## 2026-11-01 統合ブランチのローカル検証

`integration/azure-go-live` で実行。上の2026-10-30記録とは別の結果。
VM、本番、main は変更していない。Docker Desktop の使い捨て MySQL 8.0 を
127.0.0.1:13308 にだけ公開し、テスト終了後にコンテナーを削除する。

- `go build ./...` 成功。
- `go test ./...` 成功。さらに `SPACE_TEST_MYSQL_DSN` と `COURSE_PURGE_BIN` を指定した全テストも成功。
- `TestAzureMigration73To108` 成功。テスト用データの移行部分 808.591125ms。本番コピーのリハーサルではない。
- `TestPurgeIsolationAndBlobResume` 成功。授業4種・関連通知と通報を削除、添付・mentions・likes・投票の子行を削除。
  時間割、授業、授業ルーム、DM と DM 添付、共有media、同じ番号の別対象の通知・通報は保持。
  Blob はスタブで検証。実ストレージは操作していない。
- `TestCourseSearchAndAdminCountsWithSyncColumns` 成功。検索の列数、廃止除外、同期メタデータ、
  メッセージ数（時間割登録との JOIN で増幅しないこと）、昇降順ソートを検証。
- `TestAdminPostsAndOwnersAfterAccountSplit` 成功。user_accounts からの名前・account_id 検索とオーナー取得を検証。
- 既存管理者削除テスト2件は、認証済み管理者の context を fixture に補って成功。実装の認可は維持。
- `python3 -m unittest discover -s scripts -p 'test_*.py'` 4件成功。
- gqlgen は `make generate`、クライアントは `npm run codegen` で再生成。
- クライアントは `npm ci` 後の `npm run build` 成功。Playwright 8件成功
  （追加した授業同期・集計ソートとフィルター2件、既存ログイン画面等5件、refresh競合1件）。
  管理画面はモック API、その他もローカル画面のみ。実ログインやユーザー／管理者の全 E2E は未実施。
- サーバー／クライアント両方を `origin/Develop` および `origin/dev_new` と比較。
  Develop の実名表示・アカウント分割・授業同期と廃止表示、dev_new の管理画面・投稿管理・
  コミュニティ管理・ソート・メッセージ数を保持。授業一覧の年度・学期・曜日フィルターは新しいチャット管理画面へ移設。
- 匿名表示用の旧フィールド名2つが両リポジトリの統合済みソースにないことを確認。

本番コピーのリハーサル、実Blobの削除確認、実ログイン・DM復号・SSE/WebSocket、VMデプロイは今回も未実施。
到達バージョンの差分一覧は `azure-cutover.md` の統合時の表を参照。
