#!/usr/bin/env bash
# CloudFront がオリジンへ繋ぐときの送信元の一覧を、AWS の公開情報から作り直す。
# 一覧が古くなっても、新しい範囲から来た利用者が CloudFront の IP で数えられるだけで、
# 偽の IP を名乗られることはない。たまに更新すれば足りる。
set -euo pipefail

cd "$(dirname "$0")/.."
out=internal/clientip/cloudfront_ranges.txt

curl -fsS https://ip-ranges.amazonaws.com/ip-ranges.json | python3 -I -c '
import json, sys
d = json.load(sys.stdin)
v4 = sorted({p["ip_prefix"] for p in d["prefixes"] if p["service"] == "CLOUDFRONT_ORIGIN_FACING"})
v6 = sorted({p["ipv6_prefix"] for p in d["ipv6_prefixes"] if p["service"] == "CLOUDFRONT_ORIGIN_FACING"})
if not v4:
    sys.exit("no CLOUDFRONT_ORIGIN_FACING prefixes")
print("# CloudFront がオリジンへ繋ぐときの送信元（CLOUDFRONT_ORIGIN_FACING）。")
print("# scripts/update_cloudfront_ranges.sh で作り直す。createDate", d["createDate"])
print("\n".join(v4 + v6))
' > "$out.tmp"
mv "$out.tmp" "$out"
echo "updated $out ($(grep -vc '^#' "$out") prefixes)"
