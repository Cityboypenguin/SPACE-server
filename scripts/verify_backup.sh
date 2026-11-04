#!/usr/bin/env bash
# 取ったバックアップが本当に戻せるかを確かめる。
#
# 取れたことは戻せることの証明にならない。mysqldump は途中で失敗しても
# 終了コードが 0 になる経路があり、壊れたダンプは戻すときまで気づけない。
# ここでは捨てるコンテナーに実際に流し込んで、スキーマと件数を見る。
#
# 本番には一切触らない。
#   - DB へ接続しない（イメージ名も docker images から取る）
#   - 本番のボリュームを使わない（tmpfs に置いて終わったら捨てる）
# なので営業時間中に走らせても影響しない。
set -euo pipefail

DEST_DIR=${DEST_DIR:-/var/backups/space}
NAME=space-restore-check

# グロブは sudo より前に展開されるので、700 の root ディレクトリでは失敗する。
# 探索そのものを root にやらせる。
DUMP=${1:-$(sudo find "$DEST_DIR" -maxdepth 1 -name 'space-*.sql.gz' -printf '%T@ %p\n' | sort -rn | head -1 | cut -d' ' -f2-)}
[ -n "$DUMP" ] || { echo "バックアップが無い"; exit 1; }
echo "検証対象: $(basename "$DUMP") ($(sudo du -h "$DUMP" | cut -f1))"

IMAGE=$(sudo docker images --format '{{.Repository}}:{{.Tag}}' | grep -E '(^|/)mysql:8' | head -1)
[ -n "$IMAGE" ] || { echo "mysql のイメージが無い"; exit 1; }

echo "--- 捨てるコンテナーを立てる ---"
sudo docker rm -f "$NAME" >/dev/null 2>&1 || true
sudo docker run -d --name "$NAME" \
  -e MYSQL_ROOT_PASSWORD=verifyonly \
  --tmpfs /var/lib/mysql:rw,size=2g \
  "$IMAGE" --skip-log-bin >/dev/null
trap 'sudo docker rm -f "$NAME" >/dev/null 2>&1 || true' EXIT

# mysqladmin ping では足りない。初期化中の一時サーバーにも応答されてしまい、
# その間 root にはまだパスワードが無いので、後続の復元だけが Access denied で落ちる。
# 認証が通る問い合わせが成功するまで待つ。
READY=no
for i in $(seq 1 90); do
  if sudo docker exec -e MYSQL_PWD=verifyonly "$NAME" mysql -uroot -N -B -e 'SELECT 1' >/dev/null 2>&1; then
    READY=yes; break
  fi
  sleep 2
done
[ "$READY" = yes ] || { echo "受け付ける状態にならなかった"; sudo docker logs --tail 20 "$NAME"; exit 1; }

echo "--- 復元する ---"
sudo gzip -dc "$DUMP" | sudo docker exec -i -e MYSQL_PWD=verifyonly "$NAME" mysql -uroot
echo "復元できた"

q() { sudo docker exec -e MYSQL_PWD=verifyonly "$NAME" mysql -uroot -N -B space -e "$1"; }

echo "--- スキーマ ---"
printf 'テーブル %s / 外部キー %s / 索引 %s\n' \
  "$(q "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='space' AND table_type='BASE TABLE';")" \
  "$(q "SELECT COUNT(*) FROM information_schema.referential_constraints WHERE constraint_schema='space';")" \
  "$(q "SELECT COUNT(DISTINCT table_name, index_name) FROM information_schema.statistics WHERE table_schema='space';")"

echo "--- 件数（復元後の実数。table_rows は概算なので使わない）---"
# SQL 側で GROUP_CONCAT すると group_concat_max_len で黙って切られて壊れた SQL に
# なるので、表の一覧だけ受け取って組み立てはこちらでやる。
TABLES=$(q "SELECT table_name FROM information_schema.tables WHERE table_schema='space' AND table_type='BASE TABLE' ORDER BY table_name;")
SQL=$(while read -r t; do printf 'SELECT "%s" t, COUNT(*) n FROM `%s` UNION ALL ' "$t" "$t"; done <<< "$TABLES")
SQL=${SQL% UNION ALL }
q "$SQL" | sort -k2 -rn | head -8
echo "合計 $(q "$SQL" | awk '{s+=$2} END {print s}') 行"

echo "--- 中身の抜き取り ---"
q "SELECT CONCAT('users ', COUNT(*), ' 件') FROM users;"
q "SELECT CONCAT('messages ', COUNT(*), ' 件 / 本文が入っている行 ', SUM(content IS NOT NULL AND content<>'')) FROM messages;"
q "SELECT CONCAT('media ', COUNT(*), ' 件'), CONCAT('  message-media/ ', SUM(storage_key LIKE 'message-media/%')), CONCAT('  media/ ', SUM(storage_key LIKE 'media/%')), CONCAT('  avatars/ ', SUM(storage_key LIKE 'avatars/%')) FROM media;"

echo "--- 後片付け（trap で捨てるコンテナーを消す）---"
