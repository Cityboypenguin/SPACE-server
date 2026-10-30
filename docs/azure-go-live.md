# Azure VM 本番運用の作業指示

2026-10-30 時点の確認結果に基づく。作業前に実物を確認し直すこと。
`docs/azure-cutover.md` は旧計画（NS を先に移す案）を含むので、矛盾する場合はこの文書を優先する。

## 方針

- 授業チャットの非匿名化と同時に、本番を AWS から Azure VM へ移す。
- 移行は AWS のメンテナンス中に行う。
- 過去の授業ルームの投稿はすべて削除する。
- `Azure` ブランチを `main` へ入れたものを VM で動かす。

## 禁止事項

人間の明示的な指示があるまで、次を実行しない。

- サーバーの `main` への push やマージ。`deploy.yml` が AWS 本番へ自動デプロイし、起動時に
  マイグレーション `074`/`075` が匿名ラベルのテーブルを削除する。ラベルは戻らない。
- クライアントの `main` への push やマージ。S3 と Static Web Apps の両方へ配信される。
- AWS 本番の DB、S3、CloudFront、Route 53、お名前.com の変更。
- VM の `.env` の秘密情報の変更。`MESSAGE_ENCRYPTION_KEY`、`OPAQUE_ID_SECRET`、
  `ACTIVITY_ARCHIVE_HMAC_KEY`、`JWT_SECRET` は AWS と一致している必要がある。
- `31843d4` の丸ごとの revert。127 ファイルの混在コミットで、匿名以外の修正も含む。
- 秘密情報の値の表示やコミット。

## 実装タスク

### 1. サーバーの Azure デプロイ

`.github/workflows/` に Azure VM 向けのワークフローを作る。

- ACR（`20260926project202609cr.azurecr.io/space-server`）へコミットハッシュのタグで push する。
- VM（`/opt/space`）の `ACR_IMAGE` を更新し、`docker compose` で app を入れ替える。
- 失敗したら前のタグに戻せるようにする。
- `deploy.yml`（AWS 向け）は切り替え完了後に削除する。それまでの停止は人間が行う。

必要な GitHub の Secrets を一覧にして人間へ渡す。登録は人間が行う。

### 2. nginx の設定をリポジトリへ反映する

VM の `/etc/nginx/conf.d/space.conf` は certbot が更新済みで、リポジトリの
`deploy/azure/nginx.conf` は古い。VM の内容をもとに次を揃える。

- `server_name api.senshu-universe.com`、`listen 443 ssl`、証明書のパス。
- 本番切り替え時に使う 80 番から HTTPS へのリダイレクトと HSTS を、適用手順とともに用意する。
  現在 80 番はテスト用に残している。

### 3. 匿名投稿の削除スクリプト

`scripts/` に、マイグレーション後の DB に対して実行するスクリプトを作る。既定はドライランとし、
件数の表示と、明示的なフラグがあるときだけの削除にする。

対象は `rooms.type = 'course'` のルームの `messages`、`questions`、`answers`、`polls`。
2026-10-30 の本番の件数はそれぞれ 60、3、4、3。

- 子テーブルは CASCADE で消える（`message_media`、`message_mentions`、`question_media`、
  `answer_media`、`answer_likes`、`poll_options`）。
- `notifications` と `user_reports` は `target_id` で参照し、FK がないので別に消す。
- `media` の行と Blob の実体も消す。他の投稿から参照されているものは消さない。
- `space-avatars` は匿名読み取りが可能なので、実体の削除を確認する。
- Blob の論理削除は 30 日間有効。

### 4. データ移行スクリプト

`scripts/` に移行の各段階を分けて用意し、それぞれ途中で止まれるようにする。順序は次のとおり。

1. AWS で `mysqldump --single-transaction --default-character-set=utf8mb4` を実行する。
2. VM の app を止め、DB を作り直してダンプを入れる。この時点のバージョンは 73。
3. VM の app を起動し、バージョン 102 まで進んだことを確認する。
4. タスク 3 のスクリプトで投稿を削除する。
5. 画像の差分を移す。
6. `scripts/migrate_message_media.sh` を実行する。
7. 件数を照合する。

文字化けを防ぐため、MySQL の接続には必ず `--default-character-set=utf8mb4` を付ける。

`space-activity-archives` を S3 から移す必要があるか調べ、必要なら手順に加える。

### 5. Redis の扱いを調べる

`compose.azure.yaml` によると、Redis には失効トークンとワンタイムパスワードが入っている。
AWS の Redis を移さない場合に失効トークンが有効に戻るか、コードで確認し、移す方法または
代替案を提示する。移す場合、メンテナンスフラグを持ち込まないようにする。

### 6. 外部からの死活監視

`space-healthcheck.sh` は VM 上で動くので、VM 自体が止まると通知できない。Azure の外から
`https://api.senshu-universe.com/` を監視する案を提示する。

### 7. 手順書の更新

`docs/azure-cutover.md` をこの方針に合わせて書き直す。

- API を先に移す。CloudFront の `/query` と `/events` の接続先を `api.senshu-universe.com` に変える。
- 画面と DNS をあとから移す。Route 53 の別名は Static Web Apps を指せないので、`@` は
  Azure DNS の別名を使い、NS を Azure DNS へ移す。NS の TTL は 2 日なので、その間は
  CloudFront を残す。
- DB サイズは 29.7MB（手順書の 1.7MB は古い）。

## リハーサル

本番切り替えの前に、本番 DB のコピーで次を確認して結果を報告する。

- 73 から 102 までのマイグレーションの成否と所要時間。
- 削除スクリプトの件数と結果。
- ログイン、投稿、画像、アップロード、DM と添付の復号、SSE、WebSocket。

## 人間が行う作業

- VM を今のうちにメンテナンスモードにする。VM の DB は本番の古いコピーの可能性が高く、
  実名表示で外部から接続できる。
- GitHub の Secrets と変数の登録。`VITE_API_URL=https://api.senshu-universe.com/query`。
- サーバーとクライアントの `main` へのマージと、`Deploy to ECS` ワークフローの停止。
- Static Web Apps の独自ドメイン登録と Azure ゾーンの `@`、`www`、ACM の CNAME 2 件の追加。
- お名前.com の NS の変更と、AWS のメンテナンスの有効化。
- `ALLOWED_ORIGINS` の整理。`http://20.243.81.35` は切り替え時に外す。
- 過去の投稿の削除と実名表示についての利用者への告知。
- 22 番の接続元を絞るかの判断。現在はインターネットから接続でき、fail2ban で保護している。

## 確認済みで作業不要なもの

- 秘密鍵 4 つが AWS と一致。
- HTTPS と証明書の自動更新。
- 3306、6379、8080 は外部から接続不可。
- Docker の自動再起動、fail2ban、自動アップデート、スワップ。
- DB の毎日のバックアップと Blob への複製（ログで確認）。
- 2 vCPU、メモリ 7.7GB、ディスク空き 20GB。
