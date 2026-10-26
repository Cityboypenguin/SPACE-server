#!/usr/bin/env bash
# マネージド ID だけで ACR へ docker login する。VM の上で root として走らせる。
#
# それまでは手元で `az acr login --expose-token` して得た資格情報を VM へ
# 貼っていたが、あれは3時間で切れるのでデプロイのたびに人手が要った。
# VM に AcrPull が付いたので、VM 自身の ID で取り直せる。
#
# 手順は Azure の規定どおり3段:
#   1. インスタンス メタデータから AAD のアクセストークンを取る
#   2. ACR の /oauth2/exchange でリフレッシュトークンに交換する
#   3. それを docker login のパスワードとして渡す
#      （利用者名はゼロ埋めの GUID 固定。ACR の決まり）
#
# トークンは出力しない。docker へは標準入力で渡す。
set -euo pipefail

REGISTRY="${REGISTRY:-20260926project202609cr.azurecr.io}"
NULL_GUID=00000000-0000-0000-0000-000000000000

AAD=$(curl -s -m 10 -H Metadata:true \
  "http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https%3A%2F%2Fmanagement.azure.com%2F" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"])')
[ -n "$AAD" ] || { echo "マネージド ID のトークンが取れない（VM に ID が付いているか）"; exit 1; }

# テナント ID はトークンの中に入っているので、そこから取る（問い合わせが1回減る）。
# 署名は検証しない。使うのは交換先の指定だけで、不正なら交換が失敗する。
TENANT=$(python3 - "$AAD" <<'PY'
import base64, json, sys
payload = sys.argv[1].split('.')[1]
payload += '=' * (-len(payload) % 4)
print(json.loads(base64.urlsafe_b64decode(payload))['tid'])
PY
)

REFRESH=$(curl -s -m 15 -X POST "https://$REGISTRY/oauth2/exchange" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=access_token&service=$REGISTRY&tenant=$TENANT" \
  --data-urlencode "access_token=$AAD" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin).get("refresh_token",""))')
[ -n "$REFRESH" ] || { echo "ACR のトークン交換に失敗した（AcrPull が付いているか）"; exit 1; }

printf '%s' "$REFRESH" | docker login "$REGISTRY" --username "$NULL_GUID" --password-stdin
