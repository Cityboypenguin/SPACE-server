# AWS → Azure VM 本番切り替え

2026-10-30。`azure-go-live.md` の禁止事項を優先する。本番 DB は約29.7MB。
この手順書は人間がレビューして実行するためのもの。作業ブランチでは本番を変更しない。
サーバー・クライアントの main の push/merge、AWS、DNS、VM の秘密情報の変更は人間が行う。
`31843d4` は混在コミットのため丸ごと revert しない。

## 事前準備とデプロイ

VM は人間が直ちにメンテナンスにする。古い本番コピーを実名表示で公開しない。
メンテナンスフラグは Redis とプロセス内にある。全プロセスの状態を確認する。
AWS の `Deploy to ECS` は人間が停止し、サーバー・クライアントを main に入れる。
クライアントの main は S3 と SWA の両方に配信されるため、無影響とは扱わない。
AWS 向け `.github/workflows/deploy.yml` は切替完了後の別変更で削除する。

Azure ワークフローは当面 `workflow_dispatch` だけで起動する。main に入る前は Actions の
選択肢に出ない場合があるため、レビュー中はデプロイしない。GitHub Environment
`azure-production` に承認者を設定し、切替日を決めて人間が承認する。
登録する GitHub Secrets（値の登録は人間が行う）:

| Secret | 内容 |
| --- | --- |
| `AZURE_ACR_USERNAME` | 対象 ACR の push 権限を持つ専用サービスプリンシパル ID |
| `AZURE_ACR_PASSWORD` | その資格情報。ACR 全権の管理者キーは使わない |
| `AZURE_VM_HOST` | VM ホスト名または IP |
| `AZURE_VM_USER` | SSH ユーザー（現在 azureuser） |
| `AZURE_VM_SSH_KEY` | 専用 SSH 秘密鍵 |
| `AZURE_VM_KNOWN_HOSTS` | 別経路で指紋を照合したホスト鍵の known_hosts 行 |

クライアントの GitHub 変数 `VITE_API_URL=https://api.senshu-universe.com/query` を人間が登録して再ビルドする。
VM の SSH 接続元制限と GitHub runner の接続経路を事前に確認する。VM ユーザーには
`/opt/space` への配置と Docker 実行に必要な sudo 権限が要る。
VM のマネージド ID には AcrPull を用意する。DB/Redis は再作成せず app だけ入れ替える。
`.env` は表示せず `ACR_IMAGE` の一行だけを原子的に変更する。

ワークフローは `20260926project202609cr.azurecr.io/space-server:<40桁のコミットSHA>` を push する。
失敗時は直前のアプリタグに戻す。成功後の旧タグは `/opt/space/.previous-acr-image` に残る。
手動の再デプロイは `sudo bash /opt/space/scripts/deploy_azure.sh <SHA付きイメージ>`。
旧タグが短縮SHAなら、対応する完全SHAを特定して先に ACR でタグ付けする。
**アプリの切り戻しはDBを戻さない。074/075 の匿名ラベルや削除した投稿は復元しない。**
マイグレーション前に v73 のバックアップを保管し、互換性のない旧アプリへの切戻しは避ける。
デプロイ確認は `/` の200、またはメンテナンスを示す503を受け入れる。業務機能は別途確認する。

## nginx

`deploy/azure/nginx.conf` は実機の `/etc/nginx/conf.d/space.conf` を読み取り照合した
80/443 両待受設定。`nginx.production.conf` は本番用の80→HTTPS転送とHSTSを加えた設定。
証明書は `/etc/letsencrypt/live/api.senshu-universe.com/`、certbot の include も維持する。
本番HTTPSの確認後、人間が設定をバックアップして本番用ファイルを配置し、
`sudo nginx -t && sudo systemctl reload nginx` を実行する。検証失敗時はreloadせず旧ファイルへ戻す。
HTTPの301、HTTPSの証明書チェーンと HSTS、SSE、WebSocket、certbot の更新を確認する。
HSTS は API ホストだけに限定し includeSubDomains/preload は付けない。

## 移行の順序（AWS メンテナンス中）

各スクリプトは1段階だけ実行して止まる。全段階の成功を確認してから次へ進む。
本番コピーを扱うディレクトリは0700、ファイルは0600、暗号化ディスク上に置く。
ログ、SQL、削除記録、Redisエクスポートをリポジトリや Actions artifact へアップロードしない。
シェルの `set -x` や `.env` の表示は禁止。MySQL CLI はすべて utf8mb4 を明示する。

1. 人間がAWSのメンテナンスを有効にし、すべての書込み経路とバックグラウンドジョブを停止する。
   Redis の失効情報も最終スナップショットを採る（後述）。AWS の DB/S3 は削除しない。
   AWS側で `SOURCE_DB_HOST`、`SOURCE_DB_USER`、`SOURCE_DB_NAME` と接続資格情報を安全に設定し、
   `scripts/azure_migration.sh dump /secure/final.sql` を実行する。
   `mysqldump --single-transaction --default-character-set=utf8mb4` を使用する。
   DDLを同時実行しない。SQLと `.sha256` を暗号化経路でVMへ転送する。
   `scripts/azure_migration.sh source-counts > /secure/before.tsv` でv73の件数も保存する。
2. VMで `sudo bash scripts/azure_migration.sh restore /secure/final.sql --confirm-replace-vm-db`。
   appを停止しVMのDBだけを再作成する。ダンプのハッシュと clean v73 を確認する。
   実行前にVMのバックアップも確保する。間違ったVMで実行しない。
3. VMを外部から遮断したまま `sudo bash scripts/azure_migration.sh migrate --confirm-migrate`。
   app起動でマイグレーションし、clean v102 と所要秒数を確認してappを停止する。
   dirtyなら停止して調査し、versionの強制変更はしない。
4. 下記の投稿削除を実行する。DB件数とBlob削除確認まで成功させる。件数照合用に削除記録を保管する。
5. `scripts/sync_azure_objects.py` で画像の差分を運ぶ。既定は候補件数のみ。
   `--bucket <AWS公開bucket> --container space-avatars --account <account>` に加え、
   **必ず `--exclude-manifest /secure/course-purge.json`** を渡して削除済み画像の再投入を防ぐ。
   確認後に `--apply` を付ける。S3は読取りのみ。アップロード後は取得してSHA256を検証する。
   同じキーでも内容が変わっていれば更新する。宛先にだけあるBlobは消さない。
   private bucket も後述のとおり移す。画像のContent-Typeを維持する。
6. `scripts/migrate_message_media.sh` の下見を確認し、その後 `MODE=run` でDM添付を非公開へ移す。
   `space-message-media` の匿名読取りが無効なことを確認する。
7. `sudo bash scripts/azure_migration.sh counts > /secure/after.tsv` を保存し、
   `python3 scripts/verify_azure_counts.py --before /secure/before.tsv --after /secure/after.tsv --manifest /secure/course-purge.json` で削除件数との差分を照合する。
   授業投稿はすべて0、その他は削除記録と差分が一致すること。
   DM添付の公開キーが0で、コピー・参照変更・公開側削除が完了していることを確認する。
   privateアーカイブの件数・バイト数・内容ハッシュとDB参照を照合する。
   Redis失効情報を取り込み、VM側のメンテナンス設定を独立に確認してからappを起動する。

メンテナンスはバックグラウンド処理の停止を保証しない。初回のマイグレーション起動は
外部から遮断し、スケジューラーのログを確認する。コピー中はappを停止したままにする。
`migrate_message_media.sh` はDBとBlobに接続できる作業端末で実行する。他の移行スクリプトと
同時実行しない。移行中にデプロイワークフローを起動しない。

## 授業投稿の削除

`go build -o /secure/coursepurge ./scripts/coursepurge` で作成する（VM向けは
`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`）。接続先 `DB_HOST`/`DB_PORT`/`DB_NAME`/
`DB_USER`/`DB_PASSWORD` を安全に環境変数で渡す。DBは clean v102 が必要。
Blob削除は Azure CLI のログイン済み作業者で実行し、`AZURE_STORAGE_ACCOUNT_NAME`、
必要ならコンテナー名の環境変数も指定する。値をターミナルへ出さない。

```bash
/secure/coursepurge                         # ドライラン（既定）
/secure/coursepurge --apply --confirm-app-stopped --manifest /secure/course-purge.json
# DBコミット後、Blob処理が中断した場合
/secure/coursepurge --resume-blobs --confirm-app-stopped --manifest /secure/course-purge.json
```

2026-10-30の本番参考値は messages=60、questions=3、answers=4、polls=3。実際の値を確認する。
回答は質問→授業ルームで判定し、DM・コミュニティ投稿は残す。
notifications/user_reports は target_type と target_id の両方で削除し、同じ番号の別投稿は残す。
子テーブルの CASCADE を使用する。投稿・プロフィール・コミュニティから参照されるmediaは保護する。
別のmedia行が同じstorage_keyを使う場合や、規約・活動アーカイブの参照もBlob削除から保護する。

削除前に対象接続先・DB名・削除件数・Blobキーを含む0600のJSON記録を排他的に作成してからDBをコミットする。同じファイルは上書きしない。
DBの途中失敗はロールバックする。記録があってDBのmedia行が残る場合、Blob再開は拒否する。
DB失敗時は件数を調べ、記録を別名で保存してから新しい記録名でDB削除をやり直す。
Blob失敗時は記録を使って再開する。削除後に live Blob の不存在を API で検証する。
公開 `space-avatars` のURLも資格情報なしで404になることを確認する（CDNがあればキャッシュも確認）。
Blob の論理削除は30日間残る。直ちに物理消去されたとは扱わず、30日後に保持状態を再確認する。

## 活動アーカイブ

`internal/activityarchive/archiver.go` は既に集計元DBの行をアーカイブ確定後に削除する。
したがって `space-activity-archives` は空と確認できる場合を除き移行が必要。
DBの `user_activity_archives.object_key` と元の private bucket
（AWS の `MINIO_PRIVATE_BUCKET` 設定で特定）を読取りで照合する。
`analytics/user_activity_hours/` の参照オブジェクトが1つでもあれば、DBだけでは復元できない。
private bucketには規約等も含みうるため、対象prefixに漏れがないようオブジェクト一覧を確認する。

ステップ5で同じコピー処理を `--bucket <AWS private bucket> --container space-activity-archives`
として実行する。署名URLをログへ出さない。アーカイブのバイト列と件数・ハッシュを照合し、
コンテナーの匿名読取りが無効なことを確認する。`ACTIVITY_ARCHIVE_HMAC_KEY` は変更しない。
空で移行不要とする場合も、元DB参照0と元private bucketの対象件数0の両方を記録する。
本作業では本番のprivate bucketの実件数は未取得のため、移行不要とは判定していない。

## Redis

`infra/redis/revoked_token_repository.go` は `revoked:<token>` の存在で失効を判定し、
リフレッシュトークンの一度限りの消費も同じprefixで管理する。
`usecase/user/refresh_user_token.go` と `internal/auth` はこれを参照する。
JWT_SECRET を維持してRedisを空にすると、期限内の失効・消費済みトークンが再利用できる。
OTP再送だけでは解決しない。秘密鍵変更は禁止されているので、失効情報だけを移す方式を推奨する。

AWSの書込み・ログアウト・refreshを止めた後、AWS Redisへの安全な接続環境で
`go run scripts/migrate_revocations.go --export /secure/revocations.json`。
`REDIS_URL` は資格情報を含み得るためシークレットとして環境から渡し、表示しない。
VM Redisへの接続に替えて `--import /secure/revocations.json` で下見し、
`--import /secure/revocations.json --apply` で取り込む。ファイルは暗号化経路で移す。
元の絶対期限を維持し、期限切れは除外、宛先の失効期間を短縮しない。
`maintenance:enabled`、OTP、パスワードリセット、ロックはコピーしない。
メンテナンスはVMで別途設定し、appを再起動して読み込ませる。

移さない代替案はすべての既存トークンが自然失効するまでサービスを止めること。
アクセストークンだけでなくリフレッシュの最大期限まで待つ必要があり、現実的には失効情報の移行を選ぶ。
OTPとパスワードリセットの途中操作は再実行を案内する。
切替中にAWSが再びトークンを発行・消費するとスナップショットが古くなるため、その場合は取り直す。

## API を先に切り替える

DB移行とリハーサル成功後、人間が CloudFront の `/query` と `/events` のoriginを
`api.senshu-universe.com` に切り替える。画面は当面CloudFront/S3を使う。
originのHTTPS、証明書名、Hostヘッダ、Authorization・Cookie・query string の転送、
キャッシュ無効、SSEの応答タイムアウトを確認する。WebSocketのUpgradeも確認する。
フロントが他のAPIパスを使用していないか確認し、必要なbehaviorも同じoriginへ揃える。
メンテナンス解除は人間が実施し、ログイン、投稿、画像、アップロード、DM復号、SSE、WebSocketを確認する。
`ALLOWED_ORIGINS` からテスト用 `http://20.243.81.35` を切替時に外すのも人間が行う。

## 画面と DNS は後から

SWAの既定ホストで動作確認し、独自ドメインの登録・検証TXTを人間が用意する。
Route 53 の別名ではSWAを指せないので、Azure DNS の `@` の別名を使う。
Azureゾーンに `@`→SWA、`www`→SWA既定ホストのCNAME、`api` の現行値、
ACM検証用CNAME2件、およびその他の既存レコードを漏れなく用意し、Azure NSを直接指定して検証する。
CloudFront のIPを `@` の固定Aレコードに書かない。
[Azure公式のapex設定](https://learn.microsoft.com/en-us/azure/static-web-apps/apex-domain-azure-dns)を参照。

その後、人間がお名前.comでNSをAzure DNSへ変更する。NSのTTLは2日なので、
少なくとも2日間は旧Route 53、CloudFront、S3、証明書を維持する。
キャッシュの状態により古いCloudFront画面と新しいSWA画面の両方が使われる期間を想定する。
両方からAzure APIへの接続を確認する。NS変更を60秒で戻せるとは扱わない。

## 外部監視

VM内の `space-healthcheck.sh` に加えて、Azure外の
[Cloudflare Health Checks](https://developers.cloudflare.com/health-checks/) で
`https://api.senshu-universe.com/` を監視する案。
HTTP 200、本文 `test message: Hello from SPACE Server!`、TLS検証、接続タイムアウト10秒を条件とし、
60秒間隔・連続3回失敗で通知する設定を目標に、契約の利用可能設定を確認して人間が登録する。
503のメンテナンスは監視側で時間指定の抑制にする。VM停止・DNS障害・証明書失効も検出対象。
通知はVMに依存しないメール等へ送る。停止と復旧の通知が届くことを検証する。
自動復旧やAWSへの自動切替は設定しない。rootはDB/Redisの完全性を確認するエンドポイントではないため、
VM内のDB/Redis監視と組み合わせる。

## リハーサルと判定

最新本番DBのコピーと、独立DB・private Blob・Redisを使い、公開を遮断して手順1〜7を試す。
本番宛先を検証環境から共有しない。未実施の検証を成功として記録しない。
`azure-validation.md` に実施済みと未実施を分けて記録する。

| 確認 | 合格条件 |
| --- | --- |
| マイグレーション | clean 73→102、dirtyなし、所要秒数を記録 |
| 削除 | 下見と削除件数一致、授業4種0、DM等残存、共有media保持、公開Blob404 |
| ログイン/refresh | 正常ログイン、移行した失効トークンは拒否 |
| 投稿/画像/アップロード | 表示、保存、アバター・質問回答添付が成功 |
| DM | 既存本文と暗号化添付の復号、新規送受信、公開URLでは読めない |
| SSE/WebSocket | 接続とイベント送受信、再接続が成功 |
| アーカイブ | DB参照先の全オブジェクト存在とバイト列一致 |

## 切戻し

Azureへの書込みを始める前なら、APIのCloudFront originをAWSへ戻し、元DBと元アプリの組合せを維持する。
Azureで書込み後はAWSへ戻すだけでは投稿が失われる。書込みを止め、Azureのデータを保全して差分を調整する。
移行後のv102DBをv73アプリへ直接戻さない。NS変更後は2日間の混在を前提に両経路を維持する。
AWS資源、旧証明書、Route 53の削除は安定稼働と復旧可能性を人間が確認した後に別途行う。
