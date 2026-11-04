"""e2e スクリプトの共通処理。

接続先は SPACE_E2E_URL（既定は http://localhost:8080/query）、管理者の認証情報は
SPACE_ENV_FILE（既定は .env）の INIT_ADMIN_* から読む。
"""
import json, os, time, urllib.error, urllib.request

URL = os.environ.get("SPACE_E2E_URL", "http://localhost:8080/query")
WS_URL = URL.replace("http", "ws", 1)


def gql(query, variables=None, token=None):
    req = urllib.request.Request(URL, data=json.dumps({"query": query, "variables": variables or {}}).encode(),
                                 headers={"Content-Type": "application/json", **({"Authorization": f"Bearer {token}"} if token else {})})
    # サーバーのレート制限(429)に当たったら、少し待って最大5回やり直す。
    for attempt in range(6):
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                return json.load(r)
        except urllib.error.HTTPError as e:
            if e.code != 429 or attempt == 5:
                raise
            time.sleep(2 + attempt * 2)


def ok(d):
    assert "errors" not in d, d["errors"]
    return d["data"]


def err(d):
    assert "errors" in d, f"expected an error, got {d}"
    return d["errors"][0]["message"]


def read_env():
    path = os.environ.get("SPACE_ENV_FILE", ".env")
    with open(path, encoding="utf-8") as f:
        return dict(l.strip().split("=", 1) for l in f if "=" in l and not l.lstrip().startswith("#"))
