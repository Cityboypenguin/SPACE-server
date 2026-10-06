#!/bin/bash
# DM の添付を、公開の置き場から非公開の置き場へ移す。
#
# 分離前は投稿の添付と同じ公開コンテナー（匿名読み取り可）に入っていた。
# 既存のぶんをキー付きで移し、DB の参照も書き換える。
#
# 順序が重要:
#   1. 複製（非公開側へ）
#   2. DB の storage_key を更新
#   3. 公開側のオブジェクトを削除
#
# DB を先に書き換えると、複製に失敗した時点で参照先が存在しなくなり、画像が
# 表示できなくなる。3 の失敗は無視してよい（公開側に参照されないオブジェクトが
# 残るだけ）。逆に 3 を先にやると実体を失う。
#
# 何度実行しても安全。すでに message-media/ になっている行は対象から外れる。
#
# 使い方:
#   MODE=dry  ./migrate_message_media.sh    # 対象を並べるだけ（既定）
#   MODE=run  ./migrate_message_media.sh    # 実行する
set -euo pipefail

MODE="${MODE:-dry}"
ACCOUNT="${AZURE_STORAGE_ACCOUNT:-20260925project}"
SRC_CONTAINER="${SRC_CONTAINER:-space-avatars}"
DST_CONTAINER="${DST_CONTAINER:-space-message-media}"
VM="${VM:-azureuser@20.243.81.35}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/azure_space}"

ssh_vm() { ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "$VM" "$@"; }

mysql_q() {
  # -N で見出しを落とし、タブ区切りで返す
  ssh_vm "sudo docker exec -i space-mysql-1 sh -c 'mysql -uroot -p\$MYSQL_ROOT_PASSWORD space -N -B'" <<< "$1"
}

echo "=== 対象の抽出 ==="
# message_media から辿れて、まだ公開側のキーを指しているものだけ
ROWS="$(mysql_q "
SELECT m.id, m.storage_key
FROM media m
JOIN message_media mm ON mm.media_id = m.id
WHERE m.storage_key LIKE 'media/%';
")"

if [ -z "$ROWS" ]; then
  echo "対象なし。移行済み。"
  exit 0
fi

COUNT="$(printf '%s\n' "$ROWS" | grep -c . || true)"
echo "対象: ${COUNT} 件"
echo "モード: ${MODE}"
echo

MOVED=0
FAILED=0
while IFS=$'\t' read -r MEDIA_ID OLD_KEY; do
  [ -z "${MEDIA_ID:-}" ] && continue
  # media/<owner>/<uuid>.ext → message-media/<owner>/<uuid>.ext
  NEW_KEY="message-media/${OLD_KEY#media/}"

  if [ "$MODE" != "run" ]; then
    printf '  [%s] %s\n        -> %s\n' "$MEDIA_ID" "$OLD_KEY" "$NEW_KEY"
    continue
  fi

  # 1. 非公開側へ複製（サーバー間コピー。中身は手元を通らない）
  if ! az storage blob copy start --account-name "$ACCOUNT" --auth-mode login \
        --destination-container "$DST_CONTAINER" --destination-blob "$NEW_KEY" \
        --source-container "$SRC_CONTAINER" --source-blob "$OLD_KEY" \
        --requires-sync true -o none 2>/dev/null; then
    echo "  複製に失敗: $OLD_KEY（この行は飛ばす）"
    FAILED=$((FAILED+1))
    continue
  fi

  # 2. DB の参照を書き換える
  ESCAPED_NEW="${NEW_KEY//\'/\'\'}"
  mysql_q "UPDATE media SET storage_key = '${ESCAPED_NEW}' WHERE id = ${MEDIA_ID};" >/dev/null

  # 3. 公開側を消す。失敗は無視（参照されないオブジェクトが残るだけ）
  az storage blob delete --account-name "$ACCOUNT" --auth-mode login \
     --container-name "$SRC_CONTAINER" --name "$OLD_KEY" -o none 2>/dev/null || true

  MOVED=$((MOVED+1))
  printf '  移行 %d/%d: %s\n' "$MOVED" "$COUNT" "$NEW_KEY"
done <<< "$ROWS"

if [ "$MODE" != "run" ]; then
  echo
  echo "これは下見です。実行するには MODE=run を付けてください。"
  exit 0
fi

echo
echo "=== 結果 ==="
echo "移行: ${MOVED} 件 / 失敗: ${FAILED} 件"
echo "--- 残っている公開側のキー（0 なら完了）---"
mysql_q "SELECT COUNT(*) FROM media m JOIN message_media mm ON mm.media_id=m.id WHERE m.storage_key LIKE 'media/%';"
echo "--- 非公開側のオブジェクト数 ---"
az storage blob list --account-name "$ACCOUNT" --auth-mode login \
   --container-name "$DST_CONTAINER" --query "length(@)" -o tsv 2>/dev/null
