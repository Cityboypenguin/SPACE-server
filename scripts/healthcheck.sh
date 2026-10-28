#!/usr/bin/env bash
# アプリが生きているかを調べ、状態が変わったときだけメールを出す。
# VM の root の cron から5分ごとに呼ぶ。
#
# 【状態が変わった時だけにしてある理由】
# 毎回送ると、普段の「異常なし」に埋もれて読まれなくなる。読まれない通知は
# 無いのと同じなので、落ちた時と直った時の2通だけを送る。
#
# 【このスクリプトでは気づけないこと】
# VM 自体が落ちた場合、このスクリプトも動かないので通知は出ない。
# 外からの監視が別に必要（docs/azure-cutover.md 参照）。ここで捕まえられるのは
# 「VM は生きているがサービスが死んだ」場合。
#
# 【通知先】
# /opt/space/.env の ALERT_EMAIL。未設定なら SMTP_FROM（プロジェクト自身の
# アドレス）へ送る。送信はアプリと同じ SMTP 設定を使う。
#
# 【試験送信】
#   space-healthcheck.sh --test
# 送信経路だけを単体で試す。異常が起きた時に初めて「実は届かなかった」と
# 分かるのを避けるため。状態ファイルは触らないので普段の監視に影響しない。
set -u

ENV_FILE=/opt/space/.env
STATE_FILE=/var/lib/space-healthcheck.state
DEPLOY_DIR=/opt/space
DISK_LIMIT=90

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

if [ "${1:-}" = "--test" ]; then
  TO=$(alert_target)
  if [ -z "$TO" ]; then
    log "通知先が無い（.env の ALERT_EMAIL か SMTP_FROM を設定する）"
    exit 1
  fi
  log "試験送信: $TO"
  send_mail "$TO" "[SPACE] 監視の試験送信" "これは死活監視の経路を確かめる試験送信です。
対応は不要です。

送信元: $(hostname)
時刻:   $(date -u +%Y-%m-%dT%H:%M:%SZ) (UTC)"
  exit $?
fi

problems=()

# ① 外から来るのと同じ順路で確かめる（nginx → app）。
#    app を直接叩くと nginx の不調に気づけない。
code=$(curl -s -o /dev/null -m 10 -w '%{http_code}' \
  -X POST -H 'Content-Type: application/json' \
  -d '{"query":"{__typename}"}' http://127.0.0.1/query 2>/dev/null)
if [ "$code" != "200" ]; then
  problems+=("GraphQL への要求が HTTP $code（期待 200）")
fi

# ② コンテナーが揃っているか。①が通っていても mysql が落ちかけていることがある。
for svc in app mysql redis; do
  state=$(cd "$DEPLOY_DIR" && docker compose ps --format '{{.Service}} {{.State}}' 2>/dev/null \
    | awk -v s="$svc" '$1==s {print $2}')
  if [ "$state" != "running" ]; then
    problems+=("コンテナー $svc が ${state:-見つからない}")
  fi
done

# ③ ディスク。埋まるとコンテナーが揃って止まるので、止まる前に知りたい。
used=$(df --output=pcent / 2>/dev/null | tail -1 | tr -dc '0-9')
if [ -n "$used" ] && [ "$used" -ge "$DISK_LIMIT" ]; then
  problems+=("ディスク使用率 ${used}%（上限 ${DISK_LIMIT}%）")
fi

if [ "${#problems[@]}" -eq 0 ]; then
  now=ok
else
  now=ng
fi

prev=$(cat "$STATE_FILE" 2>/dev/null || echo unknown)
printf '%s\n' "$now" > "$STATE_FILE"

if [ "$now" = "$prev" ]; then
  exit 0
fi

# 初回は覚えるだけ。導入直後に「直りました」が飛ぶのを避ける。
if [ "$prev" = "unknown" ]; then
  log "初回。状態を $now として記録した"
  exit 0
fi

TO=$(alert_target)
if [ -z "$TO" ]; then
  log "通知先が無いので送らない（状態 $prev -> $now）"
  exit 0
fi

if [ "$now" = "ng" ]; then
  subject="[SPACE] 異常を検知しました"
  body="Azure の VM 上で次の問題を検知しました。

$(printf '  * %s\n' "${problems[@]}")
検知時刻: $(date -u +%Y-%m-%dT%H:%M:%SZ) (UTC)
ホスト:   $(hostname)

確認するコマンド:
  ssh azureuser@20.243.81.35
  cd /opt/space && sudo docker compose ps
  sudo docker compose logs --tail=100 app"
else
  subject="[SPACE] 復旧しました"
  body="先の異常は解消しました。

復旧時刻: $(date -u +%Y-%m-%dT%H:%M:%SZ) (UTC)
ホスト:   $(hostname)"
fi

log "状態が $prev -> $now に変わったので通知する"
send_mail "$TO" "$subject" "$body"
