#!/usr/bin/env bash
# アプリが生きているかを調べ、直せそうなら一度だけ自分で直し、
# 状態が変わったときにメールを出す。VM の root の cron から5分ごとに呼ぶ。
#
# 【Docker の再起動方針との分担】
# コンテナーは restart=unless-stopped なので、プロセスが終了すれば Docker が
# 立て直す。ここで扱うのは「生きているのに応答しない」場合で、終了して
# いないので Docker は気づかない。
#
# 【自動復帰を app だけに限っている理由】
# mysql と redis は落ちていても自動で再起動しない。データを持っているものを
# 黙って再起動すると、壊れかけている状態を上書きして原因が分からなくなる。
# この2つは通知だけにして、人が見てから判断する。
#
# 【間隔を空けている理由】
# 直らない相手を5分ごとに再起動し続けると、復帰しないまま再起動を繰り返す。
# 症状が「時々落ちる」に見えてしまい、本当の原因が隠れる。一度試して直らな
# ければ、RESTART_COOLDOWN の間は再起動せず通知だけにする。
#
# 【状態が変わった時だけ通知する理由】
# 毎回送ると、普段の「異常なし」に埋もれて読まれなくなる。読まれない通知は
# 無いのと同じ。ただし自動復帰は状態が元に戻るため、出来事として別に通知する。
# 黙って直すと、繰り返していることに誰も気づけない。
#
# 【このスクリプトでは気づけないこと】
# VM 自体が落ちた場合、このスクリプトも動かないので通知は出ない。
# 外からの監視が別に必要（docs/azure-cutover.md 参照）。
#
# 【通知先】
# /opt/space/.env の ALERT_EMAIL。未設定なら SMTP_FROM（プロジェクト自身の
# アドレス）へ送る。送信はアプリと同じ SMTP 設定を使う。
#
# カンマ区切りで複数指定できる。空白はあっても無くてもよい。
#   ALERT_EMAIL=a@example.com,b@example.com
# To: 欄に並べるだけでよく、宛先の取り出しは python の smtplib が欄から行う
# （複数が正しく分解されることを確認済み）。
#
# 通知先は「普段見ている場所」にすること。誰も見ないアドレスに送っても、
# 異常に気づけない点は監視が無いのと変わらない。
#
# 【試験送信】
#   space-healthcheck.sh --test
# 送信経路だけを単体で試す。異常が起きた時に初めて「実は届かなかった」と
# 分かるのを避けるため。状態ファイルは触らないので普段の監視に影響しない。
set -u

ENV_FILE=/opt/space/.env
STATE_FILE=/var/lib/space-healthcheck.state
RESTART_STAMP=/var/lib/space-healthcheck.lastrestart
DEPLOY_DIR=/opt/space
DISK_LIMIT=90
RESTART_COOLDOWN=1800   # 秒。この間は再起動を試さない
RECOVER_WAIT=60         # 再起動後、応答を待つ上限（秒）

log() { printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }

read_env() {
  # 値に = を含む場合があるので最初の = だけで切る。値は表示しない。
  sed -n "s/^$1=//p" "$ENV_FILE" 2>/dev/null | tail -1
}

alert_target() {
  local to
  to=$(read_env ALERT_EMAIL)
  [ -n "$to" ] || to=$(read_env SMTP_FROM)
  printf '%s' "$to"
}

# send_mail 宛先 件名 本文
# 認証情報は環境変数で渡す。引数に置くと ps で見える。
send_mail() {
  SMTP_HOST=$(read_env SMTP_HOST) \
  SMTP_PORT=$(read_env SMTP_PORT) \
  SMTP_USERNAME=$(read_env SMTP_USERNAME) \
  SMTP_PASSWORD=$(read_env SMTP_PASSWORD) \
  SMTP_FROM=$(read_env SMTP_FROM) \
  MAIL_TO="$1" MAIL_SUBJECT="$2" MAIL_BODY="$3" \
  python3 - <<'PYEOF'
import os, smtplib, sys
from email.message import EmailMessage
from email.utils import formatdate

msg = EmailMessage()
msg["From"] = os.environ.get("SMTP_FROM") or os.environ["SMTP_USERNAME"]
msg["To"] = os.environ["MAIL_TO"]
msg["Subject"] = os.environ["MAIL_SUBJECT"]
msg["Date"] = formatdate(localtime=True)
msg.set_content(os.environ["MAIL_BODY"])

host = os.environ.get("SMTP_HOST") or "smtp.gmail.com"
port = int(os.environ.get("SMTP_PORT") or 587)
try:
    with smtplib.SMTP(host, port, timeout=20) as server:
        server.starttls()
        if os.environ.get("SMTP_USERNAME"):
            server.login(os.environ["SMTP_USERNAME"], os.environ["SMTP_PASSWORD"])
        server.send_message(msg)
except Exception as exc:
    # 例外に認証情報は載らないが、念のため種類だけを出す。
    print("送信に失敗: %s" % type(exc).__name__, file=sys.stderr)
    sys.exit(1)
print("送信した")
PYEOF
}

notify() {
  local to
  to=$(alert_target)
  if [ -z "$to" ]; then
    log "通知先が無いので送らない（件名: $1）"
    return 0
  fi
  send_mail "$to" "$1" "$2"
}

host_line() {
  printf '時刻:   %s (UTC)\nホスト: %s' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$(hostname)"
}

how_to_look() {
  cat <<'TXT'
確認するコマンド:
  ssh azureuser@20.243.81.35
  cd /opt/space && sudo docker compose ps
  sudo docker compose logs --tail=200 app
  sudo tail -50 /var/log/space-healthcheck.log
TXT
}

if [ "${1:-}" = "--test" ]; then
  TO=$(alert_target)
  if [ -z "$TO" ]; then
    log "通知先が無い（.env の ALERT_EMAIL か SMTP_FROM を設定する）"
    exit 1
  fi
  log "試験送信: $TO"
  send_mail "$TO" "[SPACE] 監視の試験送信" "これは死活監視の経路を確かめる試験送信です。
対応は不要です。

$(host_line)"
  exit $?
fi

# 外から来るのと同じ順路で確かめる（nginx → app）。
# app を直接叩くと nginx の不調に気づけない。
app_responds() {
  local code
  code=$(curl -s -o /dev/null -m 10 -w '%{http_code}' \
    -X POST -H 'Content-Type: application/json' \
    -d '{"query":"{__typename}"}' http://127.0.0.1/query 2>/dev/null)
  [ "$code" = "200" ]
}

container_state() {
  (cd "$DEPLOY_DIR" && docker compose ps --format '{{.Service}} {{.State}}' 2>/dev/null) \
    | awk -v s="$1" '$1==s {print $2}'
}

# 調べ直して problems を作る。復帰を試した後にもう一度呼ぶ。
collect_problems() {
  problems=()
  app_responds || problems+=("GraphQL への要求が 200 を返さない")
  local svc state
  for svc in app mysql redis; do
    state=$(container_state "$svc")
    [ "$state" = "running" ] || problems+=("コンテナー $svc が ${state:-見つからない}")
  done
  # ディスク。埋まるとコンテナーが揃って止まるので、止まる前に知りたい。
  local used
  used=$(df --output=pcent / 2>/dev/null | tail -1 | tr -dc '0-9')
  if [ -n "$used" ] && [ "$used" -ge "$DISK_LIMIT" ]; then
    problems+=("ディスク使用率 ${used}%（上限 ${DISK_LIMIT}%）")
  fi
}

# 自動で直して良いのは app だけ。mysql / redis / ディスクは人が見る。
only_app_is_broken() {
  local p
  [ "${#problems[@]}" -gt 0 ] || return 1
  for p in "${problems[@]}"; do
    case "$p" in
      "GraphQL への要求が 200 を返さない") ;;
      "コンテナー app が"*) ;;
      *) return 1 ;;
    esac
  done
  return 0
}

cooldown_passed() {
  local last now
  last=$(cat "$RESTART_STAMP" 2>/dev/null || echo 0)
  now=$(date +%s)
  [ $((now - last)) -ge "$RESTART_COOLDOWN" ]
}

collect_problems

recovered=no
if only_app_is_broken; then
  if cooldown_passed; then
    log "app のみ異常。再起動を試す: ${problems[*]}"
    date +%s > "$RESTART_STAMP"
    (cd "$DEPLOY_DIR" && docker compose restart app) >/dev/null 2>&1

    waited=0
    while [ "$waited" -lt "$RECOVER_WAIT" ]; do
      sleep 5
      waited=$((waited + 5))
      if app_responds; then break; fi
    done

    before="${problems[*]}"
    collect_problems
    if [ "${#problems[@]}" -eq 0 ]; then
      recovered=yes
      log "再起動で復帰した（${waited}秒）"
      notify "[SPACE] 自動復帰しました" "応答が無くなったため app を再起動し、復帰しました。
**サービスは動いていますが、原因は残っています。**ログを確認してください。

検知した内容: $before
復帰までの待ち: ${waited} 秒

$(host_line)

$(how_to_look)"
    else
      log "再起動したが直らない: ${problems[*]}"
    fi
  else
    log "冷却期間中なので再起動しない: ${problems[*]}"
  fi
fi

if [ "${#problems[@]}" -eq 0 ]; then
  now=ok
else
  now=ng
fi

prev=$(cat "$STATE_FILE" 2>/dev/null || echo unknown)
printf '%s\n' "$now" > "$STATE_FILE"

# 自動復帰は上で通知済み。状態も元に戻るので、ここでは何もしない。
[ "$recovered" = "yes" ] && exit 0

if [ "$now" = "$prev" ]; then
  exit 0
fi

# 初回は覚えるだけ。導入直後に「直りました」が飛ぶのを避ける。
if [ "$prev" = "unknown" ]; then
  log "初回。状態を $now として記録した"
  exit 0
fi

if [ "$now" = "ng" ]; then
  notify "[SPACE] 異常を検知しました" "Azure の VM 上で次の問題を検知しました。
自動では直せていません。

$(printf '  * %s\n' "${problems[@]}")
$(host_line)

$(how_to_look)"
else
  notify "[SPACE] 復旧しました" "先の異常は解消しました。

$(host_line)"
fi
