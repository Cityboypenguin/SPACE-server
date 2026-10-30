#!/usr/bin/env bash
# DM 添付: private にコピー -> DB 更新 -> 参照のない公開実体を削除。
# MODE=dry（既定）/ MODE=run。コピー済み行も再実行で公開側削除を再確認する。
set -euo pipefail
MODE=${MODE:-dry}
ACCOUNT=${AZURE_STORAGE_ACCOUNT:-20261024project202609su}
SRC_CONTAINER=${SRC_CONTAINER:-space-avatars}
DST_CONTAINER=${DST_CONTAINER:-space-message-media}
VM=${VM:-azureuser@20.243.81.35}
SSH_KEY=${SSH_KEY:-$HOME/.ssh/azure_space}
[[ "$MODE" == dry || "$MODE" == run ]] || exit 2
[[ "$SRC_CONTAINER" != "$DST_CONTAINER" ]] || exit 2
ssh_vm() { ssh -i "$SSH_KEY" -o StrictHostKeyChecking=yes "$VM" "$@"; }
mysql_q() {
  ssh_vm 'cd /opt/space && sudo docker compose --env-file .env -f compose.azure.yaml exec -T mysql sh -c '\''MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --default-character-set=utf8mb4 "$MYSQL_DATABASE" -N -B'\''' <<< "$1"
}
ROWS=$(mysql_q "SELECT DISTINCT m.id, m.storage_key FROM media m JOIN message_media mm ON mm.media_id=m.id WHERE m.storage_key LIKE 'media/%' OR m.storage_key LIKE 'message-media/%';")
[[ -n "$ROWS" ]] || { echo '対象なし'; exit 0; }
MOVED=0
FAILED=0
while IFS=$'\t' read -r MEDIA_ID OLD_KEY; do
  [[ "$MEDIA_ID" =~ ^[0-9]+$ && "$OLD_KEY" =~ ^(media|message-media)/[A-Za-z0-9/_.-]+$ ]] || { echo 'Unexpected media key' >&2; exit 2; }
  ALREADY_MOVED=false
  if [[ "$OLD_KEY" == message-media/* ]]; then
    ALREADY_MOVED=true
    NEW_KEY="$OLD_KEY"
    OLD_KEY="media/${NEW_KEY#message-media/}"
  else
    NEW_KEY="message-media/${OLD_KEY#media/}"
  fi
  if [[ "$MODE" == dry ]]; then
    printf 'media id %s: copy=%s; verify public deletion\n' "$MEDIA_ID" "$([[ "$ALREADY_MOVED" == true ]] && echo no || echo yes)"
    continue
  fi
  if [[ "$ALREADY_MOVED" == false ]]; then
    if ! az storage blob copy start --account-name "$ACCOUNT" --auth-mode login \
      --destination-container "$DST_CONTAINER" --destination-blob "$NEW_KEY" \
      --source-container "$SRC_CONTAINER" --source-blob "$OLD_KEY" \
      --requires-sync true -o none 2>/dev/null; then
      FAILED=$((FAILED+1)); continue
    fi
    # Keys are restricted to SQL-safe characters above. Existing shared references
    # to this media row must not be redirected to a private message container.
    SHARED=$(mysql_q "SELECT (SELECT COUNT(*) FROM post_media WHERE media_id=${MEDIA_ID}) + (SELECT COUNT(*) FROM question_media WHERE media_id=${MEDIA_ID}) + (SELECT COUNT(*) FROM answer_media WHERE media_id=${MEDIA_ID}) + (SELECT COUNT(*) FROM profiles WHERE avatar_media_id=${MEDIA_ID}) + (SELECT COUNT(*) FROM communities WHERE avatar_media_id=${MEDIA_ID});")
    [[ "$SHARED" == 0 ]] || { echo 'Shared media requires manual separation' >&2; FAILED=$((FAILED+1)); continue; }
    mysql_q "UPDATE media SET storage_key='${NEW_KEY}' WHERE id=${MEDIA_ID} AND storage_key='${OLD_KEY}';" >/dev/null
  fi
  PRIVATE_EXISTS=$(az storage blob exists --account-name "$ACCOUNT" --auth-mode login --container-name "$DST_CONTAINER" --name "$NEW_KEY" --query exists -o tsv)
  [[ "$PRIVATE_EXISTS" == true ]] || { FAILED=$((FAILED+1)); continue; }
  REFS=$(mysql_q "SELECT (SELECT COUNT(*) FROM media WHERE storage_key='${OLD_KEY}') + (SELECT COUNT(*) FROM terms_of_service WHERE object_key='${OLD_KEY}') + (SELECT COUNT(*) FROM user_activity_archives WHERE object_key='${OLD_KEY}');")
  if [[ "$REFS" == 0 ]]; then
    EXISTS=$(az storage blob exists --account-name "$ACCOUNT" --auth-mode login --container-name "$SRC_CONTAINER" --name "$OLD_KEY" --query exists -o tsv)
    if [[ "$EXISTS" == true ]]; then
      if ! az storage blob delete --account-name "$ACCOUNT" --auth-mode login --container-name "$SRC_CONTAINER" --name "$OLD_KEY" --delete-snapshots include -o none 2>/dev/null; then
        FAILED=$((FAILED+1)); continue
      fi
    elif [[ "$EXISTS" != false ]]; then
      FAILED=$((FAILED+1)); continue
    fi
    EXISTS=$(az storage blob exists --account-name "$ACCOUNT" --auth-mode login --container-name "$SRC_CONTAINER" --name "$OLD_KEY" --query exists -o tsv)
    [[ "$EXISTS" == false ]] || { FAILED=$((FAILED+1)); continue; }
  fi
  MOVED=$((MOVED+1))
done <<< "$ROWS"
echo "mode=$MODE verified=$MOVED failed=$FAILED"
[[ "$FAILED" == 0 ]] || exit 1
if [[ "$MODE" == run ]]; then
  REMAINING=$(mysql_q "SELECT COUNT(*) FROM media m JOIN message_media mm ON mm.media_id=m.id WHERE m.storage_key LIKE 'media/%';")
  [[ "$REMAINING" == 0 ]] || { echo 'Public message attachments remain' >&2; exit 1; }
fi
