#!/usr/bin/env bash
# MySQL の中身を1ファイルに吐き出して、古いものを捨てる。
# VM の root の cron から日次で呼ぶ。
#
# 【どこに置くか】
# まず VM のディスクに吐き、そのあと Azure Blob にも複製する。
# ディスクだけだと、ディスクごと失う事故でバックアップも一緒に消える。
#
# Blob へは VM のマネージド ID で送る（鍵をこのVMに置かない）。az CLI は
# 入れず、トークンを取って REST で PUT する。1ファイル2MB程度なので
# 1リクエストで収まる。
#
# Blob 側の世代管理はストレージアカウントのライフサイクル規則に任せる
# （30日で自動削除）。消す処理をここに書くと、消す権限もこのVMに必要になる。
#
# 【戻し方】
# 取ったものは、検証用に立てた捨てるコンテナーで必ず一度試すこと
# （scripts/verify_backup.sh がそれをやる）。本番へ戻す手順は次の通り。
#
#   1. アプリだけ止める（DB は起こしたまま）
#        cd /opt/space && sudo docker compose stop app
#   2. 戻す。ダンプは CREATE DATABASE から入っているので、
#      今の space は作り直される（戻す時点より後の変更は消える）
#        P=$(sudo sed -n 's/^DB_PASSWORD=//p' /opt/space/.env | tail -1)
#        C=$(cd /opt/space && sudo docker compose ps -q mysql)
#        sudo gzip -dc /var/backups/space/space-YYYYmmdd-HHMMSS.sql.gz \
#          | sudo docker exec -i -e MYSQL_PWD="$P" "$C" mysql -uroot
#   3. アプリを起こす
#        cd /opt/space && sudo docker compose up -d app
#
# VM のディスクから消えている場合は Blob から取る。
#   az storage blob download --account-name <アカウント> \
#     --container-name space-db-backups --name db/space-YYYYmmdd-HHMMSS.sql.gz \
#     --file ./restore.sql.gz --auth-mode login
#
# 戻すと Blob 側のファイルは戻らない（DB だけが過去に戻る）。
# DB が指しているのに実体が無いファイルが出うるので、
# 画像の欠けは戻した後に確認すること。
#
# 【失敗したときに何が起きるか】
# 途中で失敗したら、その回のファイルは .part のまま残して消す。
# 古い世代の削除は吐き出しが成功した後にしか走らないので、
# 失敗が続いても手持ちの最後に成功したぶんは消えない。
set -euo pipefail

DEPLOY_DIR=${DEPLOY_DIR:-/opt/space}
DEST_DIR=${DEST_DIR:-/var/backups/space}
KEEP_DAYS=${KEEP_DAYS:-7}

log() { printf '%s %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$*"; }
die() { log "ERROR: $*"; exit 1; }

[ -f "$DEPLOY_DIR/.env" ] || die "$DEPLOY_DIR/.env が無い"

# .env は compose 用なので、値を読むだけにして環境へ撒かない。
# クォートや空行が混ざっていてもいいように、必要な3つだけ取り出す。
read_env() {
  sed -n "s/^$1=//p" "$DEPLOY_DIR/.env" | tail -1 | sed -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'$//"
}
DB_NAME=$(read_env DB_NAME); DB_NAME=${DB_NAME:-space}
DB_USER=$(read_env DB_USER); DB_USER=${DB_USER:-root}
DB_PASSWORD=$(read_env DB_PASSWORD)
[ -n "$DB_PASSWORD" ] || die ".env に DB_PASSWORD が無い"

# コンテナー名を直書きしない。compose に today の名前を聞く
# （再作成で名前が変わってもここが追随する）。
CONTAINER=$(cd "$DEPLOY_DIR" && docker compose ps -q mysql 2>/dev/null | head -1)
[ -n "$CONTAINER" ] || die "mysql のコンテナーが見つからない（起動しているか）"

# 中身はハッシュ化した資格情報と暗号化した本文を含む。読めるのは root だけにする。
install -d -m 700 -o root -g root "$DEST_DIR"

STAMP=$(date -u '+%Y%m%d-%H%M%S')
OUT="$DEST_DIR/${DB_NAME}-${STAMP}.sql.gz"
PART="$OUT.part"
trap 'rm -f "$PART"' EXIT

log "dump 開始 db=$DB_NAME container=${CONTAINER:0:12}"

# パスワードは引数に置かない（コンテナーの ps に出る）。MYSQL_PWD で渡す。
# --single-transaction は InnoDB を止めずに一貫した状態を取るため。
docker exec -i -e MYSQL_PWD="$DB_PASSWORD" "$CONTAINER" \
  mysqldump \
    --user="$DB_USER" \
    --databases "$DB_NAME" \
    --single-transaction \
    --quick \
    --routines --triggers --events \
    --no-tablespaces \
    --default-character-set=utf8mb4 \
  | gzip -6 > "$PART"

# mysqldump は途中で失敗しても終了コードが 0 になる経路があるので、
# 中身を見て確かめる。完走したときだけ末尾にこの行が入る。
gzip -t "$PART" 2>/dev/null || die "gzip が壊れている"
gzip -dc "$PART" | tail -5 | grep -q '^-- Dump completed' \
  || die "dump が完走していない（末尾の完了行が無い）"

chmod 600 "$PART"
mv "$PART" "$OUT"
trap - EXIT
log "dump 完了 $(basename "$OUT") $(du -h "$OUT" | cut -f1)"

# 成功した後にだけ古い世代を捨てる。
DELETED=$(find "$DEST_DIR" -maxdepth 1 -name "${DB_NAME}-*.sql.gz" -mtime "+$KEEP_DAYS" -print -delete | wc -l | tr -d ' ')
log "保持 ${KEEP_DAYS}日 / 削除 ${DELETED}件 / 現在 $(find "$DEST_DIR" -maxdepth 1 -name "${DB_NAME}-*.sql.gz" | wc -l | tr -d ' ')件"

# 取り残した .part（前回の異常終了ぶん）も片付ける。
find "$DEST_DIR" -maxdepth 1 -name '*.sql.gz.part' -mtime +1 -delete

# ---- ここから Blob への複製 ----
#
# ここまでで VM のディスクには確実に残っている。以降が失敗しても、
# それは「遠くに置けなかった」だけなので、ローカルの世代は巻き戻さない。
# ただし失敗は終了コードに出す（ログを見れば分かるようにする）。

ACCOUNT=$(read_env AZURE_STORAGE_ACCOUNT_NAME)
CONTAINER="${BLOB_CONTAINER:-space-db-backups}"

if [ -z "$ACCOUNT" ]; then
  log "WARN: .env に AZURE_STORAGE_ACCOUNT_NAME が無いので Blob への複製を飛ばす"
  exit 0
fi

# マネージド ID でストレージ用のトークンを取る。
TOKEN=$(curl -s -m 10 -H Metadata:true \
  "http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https%3A%2F%2Fstorage.azure.com%2F" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin).get("access_token",""))')
if [ -z "$TOKEN" ]; then
  log "ERROR: マネージド ID のトークンが取れないので Blob へ複製できない"
  exit 1
fi

BLOB="db/$(basename "$OUT")"
# x-ms-version は AAD 認証に対応した版を明示する。古い版だと Bearer を受け付けない。
# x-ms-blob-type はブロック BLOB の PUT に必須。
CODE=$(curl -s -o /tmp/space-backup-upload.err -w '%{http_code}' -X PUT \
  -H "Authorization: Bearer $TOKEN" \
  -H "x-ms-version: 2021-08-06" \
  -H "x-ms-blob-type: BlockBlob" \
  -H "Content-Type: application/gzip" \
  --data-binary "@$OUT" \
  "https://${ACCOUNT}.blob.core.windows.net/${CONTAINER}/${BLOB}")

if [ "$CODE" = "201" ]; then
  log "Blob へ複製 ${CONTAINER}/${BLOB}"
  rm -f /tmp/space-backup-upload.err
else
  log "ERROR: Blob への複製が HTTP $CODE で失敗（ローカルの世代は残っている）"
  head -c 300 /tmp/space-backup-upload.err 2>/dev/null | tr -d '\0'; echo
  exit 1
fi
