#!/usr/bin/env bash
# VM 上で root として実行。秘密情報は変更せず ACR_IMAGE のみ更新する。
set -euo pipefail
cd "${SPACE_DEPLOY_DIR:-/opt/space}"
exec 9>.deploy.lock
flock -n 9 || { echo 'Another deployment is running' >&2; exit 1; }
image=${1:?commit-tagged ACR image required}
[[ "$image" =~ ^20260926project202609cr.azurecr.io/space-server:[0-9a-f]{40}$ ]] || exit 2
compose=(docker compose --env-file .env -f compose.azure.yaml)
previous=$(sed -n 's/^ACR_IMAGE=//p' .env)
[[ "$previous" =~ ^20260926project202609cr.azurecr.io/space-server:[A-Za-z0-9_.-]+$ ]] || { echo 'Invalid previous image' >&2; exit 2; }
bash scripts/acr_login.sh
ACR_IMAGE="$image" "${compose[@]}" pull app
set_image() {
  python3 - "$1" <<'PY'
import os, pathlib, sys, tempfile
p = pathlib.Path('.env')
lines = p.read_text().splitlines()
assert sum(line.startswith('ACR_IMAGE=') for line in lines) == 1
fd, name = tempfile.mkstemp(dir='.', prefix='.env.deploy-')
try:
    os.fchmod(fd, p.stat().st_mode & 0o777)
    with os.fdopen(fd, 'w') as f:
        f.write('\n'.join('ACR_IMAGE='+sys.argv[1] if x.startswith('ACR_IMAGE=') else x for x in lines)+'\n')
    os.chown(name, p.stat().st_uid, p.stat().st_gid)
    os.replace(name, p)
finally:
    if os.path.exists(name): os.unlink(name)
PY
}
healthy() {
  for i in {1..60}; do
    status=$(curl --silent --max-time 3 -o /tmp/space-deploy-health.$$ -w '%{http_code}' http://127.0.0.1:8080/ || true)
    if [[ "$status" == 200 ]] || { [[ "$status" == 503 ]] && grep -q '"message":"server_maintenance"' /tmp/space-deploy-health.$$; }; then
      rm -f /tmp/space-deploy-health.$$
      return 0
    fi
    rm -f /tmp/space-deploy-health.$$
    sleep 2
  done
  return 1
}
rollback() {
  trap - ERR INT TERM
  echo 'Deploy failed; restoring previous app image. Database migrations are NOT rolled back.' >&2
  set_image "$previous"
  "${compose[@]}" up -d --no-deps app
  healthy || echo 'Rollback health check failed; operator intervention required' >&2
  exit 1
}
trap rollback ERR INT TERM
set_image "$image"
"${compose[@]}" up -d --no-deps app
healthy
trap - ERR INT TERM
printf '%s\n' "$previous" > .previous-acr-image
printf 'Deployed %s\n' "$image"
