#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cp "$root/tests/report-valid.json" "$tmp/good.json"
"$root/verify-report.sh" "$tmp/good.json" >/dev/null

reject() {
  local name=$1 expression=$2
  jq "$expression" "$root/tests/report-valid.json" >"$tmp/$name.json"
  if "$root/verify-report.sh" "$tmp/$name.json" >/dev/null 2>&1; then
    echo "unexpectedly accepted $name fixture" >&2
    exit 1
  fi
}
reject missing-delete 'del(.archives[0].k6_metrics[] | select(.metric == "xp_time_to_delete"))'
reject censored '.archives[0].k6_metrics |= map(if .metric == "xp_time_to_ready" then .censored = true else . end)'
reject nonfinite '.archives[0].k6_metrics |= map(if .metric == "xp_time_to_ready" then .finite_sample_count = 0 | .non_finite_sample_count = 1 else . end)'
reject multiple-archives '.archives += [.archives[0]]'
reject checks '.checks = [] | .policy = {"filename":"policy.yaml"}'
reject wrong-status '.status = "passed"'
echo 'Report verifier fixtures passed.'
