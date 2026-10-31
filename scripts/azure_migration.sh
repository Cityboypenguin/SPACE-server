#!/usr/bin/env bash
# One stage per invocation; never advances automatically. Run on source (dump)
# or VM (restore/migrate/verify). AWS export must be run by the operator.
set -euo pipefail
umask 077
stage=${1:-help}
shift || true
compose=(docker compose --env-file /opt/space/.env -f /opt/space/compose.azure.yaml)
mysql_vm() {
  "${compose[@]}" exec -T mysql sh -c 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --default-character-set=utf8mb4 "$MYSQL_DATABASE" -N -B' "$@"
}
version() { mysql_vm <<< 'SELECT version, dirty FROM schema_migrations;'; }
case "$stage" in
  dump)
    # MYSQL_PWD or a restricted defaults file supplies credentials; no command-line password.
    dest=${1:?dump path}; [[ ! -e "$dest" ]] || exit 2
    : "${SOURCE_DB_HOST:?}" "${SOURCE_DB_USER:?}" "${SOURCE_DB_NAME:?}"
    [[ "$SOURCE_DB_NAME" =~ ^[a-zA-Z0-9_]+$ ]] || exit 2
    # Keep SQL out of logs. Failed dumps are never promoted to the final path.
    trap 'rm -f "$dest.partial"' EXIT
    mysqldump --host="$SOURCE_DB_HOST" --port="${SOURCE_DB_PORT:-3306}" --user="$SOURCE_DB_USER" \
      --single-transaction --default-character-set=utf8mb4 --set-gtid-purged=OFF --no-tablespaces \
      "$SOURCE_DB_NAME" > "$dest.partial"
    mv "$dest.partial" "$dest"
    shasum -a 256 "$dest" > "$dest.sha256"
    ;;
  source-counts)
    : "${SOURCE_DB_HOST:?}" "${SOURCE_DB_USER:?}" "${SOURCE_DB_NAME:?}"
    mysql --host="$SOURCE_DB_HOST" --port="${SOURCE_DB_PORT:-3306}" --user="$SOURCE_DB_USER" \
      --default-character-set=utf8mb4 "$SOURCE_DB_NAME" -N -B <<'SQL'
SELECT 'schema', version, dirty FROM schema_migrations;
SELECT 'users', COUNT(*) FROM users;
SELECT 'messages', COUNT(*) FROM messages;
SELECT 'questions', COUNT(*) FROM questions;
SELECT 'answers', COUNT(*) FROM answers;
SELECT 'polls', COUNT(*) FROM polls;
SELECT 'media', COUNT(*) FROM media;
SELECT 'notifications', COUNT(*) FROM notifications;
SELECT 'user_reports', COUNT(*) FROM user_reports;
SELECT 'archives', COUNT(*) FROM user_activity_archives;
SQL
    ;;
  restore)
    dump=${1:?dump path}; [[ "${2:-}" == --confirm-replace-vm-db ]] || exit 2
    cd /opt/space
    shasum -a 256 -c "$dump.sha256"
    "${compose[@]}" stop app
    "${compose[@]}" exec -T mysql sh -eu -c '
      case "$MYSQL_DATABASE" in ""|*[!a-zA-Z0-9_]*) exit 2;; esac
      export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"
      mysql -uroot --default-character-set=utf8mb4 -e "DROP DATABASE IF EXISTS \`$MYSQL_DATABASE\`; CREATE DATABASE \`$MYSQL_DATABASE\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
    '
    mysql_vm < "$dump"
    [[ "$(version)" == $'73\t0' ]] || { echo 'Expected clean version 73; app remains stopped' >&2; exit 1; }
    ;;
  migrate)
    [[ "${1:-}" == --confirm-migrate ]] || exit 2
    [[ "$(version)" == $'73\t0' ]] || exit 1
    start=$SECONDS
    "${compose[@]}" up -d app
    for i in {1..120}; do
      if [[ "$(version)" == $'102\t0' ]]; then
        echo "Migrated 73 -> 102 in $((SECONDS-start)) seconds"
        "${compose[@]}" stop app
        exit 0
      fi
      sleep 2
    done
    "${compose[@]}" stop app
    echo 'Migration failed or timed out; do not force migration version' >&2
    exit 1
    ;;
  counts)
    mysql_vm <<'SQL'
SELECT 'schema', version, dirty FROM schema_migrations;
SELECT 'users', COUNT(*) FROM users;
SELECT 'user_accounts', COUNT(*) FROM user_accounts;
SELECT 'messages', COUNT(*) FROM messages;
SELECT 'questions', COUNT(*) FROM questions;
SELECT 'answers', COUNT(*) FROM answers;
SELECT 'polls', COUNT(*) FROM polls;
SELECT 'media', COUNT(*) FROM media;
SELECT 'notifications', COUNT(*) FROM notifications;
SELECT 'user_reports', COUNT(*) FROM user_reports;
SELECT 'archives', COUNT(*) FROM user_activity_archives;
SELECT 'course_messages', COUNT(*) FROM messages m JOIN rooms r ON r.id=m.room_id WHERE r.type='course';
SELECT 'course_questions', COUNT(*) FROM questions q JOIN rooms r ON r.id=q.room_id WHERE r.type='course';
SELECT 'course_answers', COUNT(*) FROM answers a JOIN questions q ON q.id=a.question_id JOIN rooms r ON r.id=q.room_id WHERE r.type='course';
SELECT 'course_polls', COUNT(*) FROM polls p JOIN rooms r ON r.id=p.room_id WHERE r.type='course';
SQL
    ;;
  *) echo 'Stages: dump FILE | restore FILE --confirm-replace-vm-db | migrate --confirm-migrate | source-counts | counts';;
esac
