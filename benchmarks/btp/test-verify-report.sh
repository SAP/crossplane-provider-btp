#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
"$root/verify-report.sh" "$root/tests/report-valid.json" >/dev/null

reject() {
  local name=$1 expression=$2
  jq "$expression" "$root/tests/report-valid.json" >"$tmp/$name.json"
  if "$root/verify-report.sh" "$tmp/$name.json" >/dev/null 2>&1; then
    echo "unexpectedly accepted $name fixture" >&2
    exit 1
  fi
}
kinds=(Subaccount Directory Entitlement DirectoryEntitlement SubaccountApiCredential)
for kind in "${kinds[@]}"; do
  key=$(printf '%s' "$kind" | tr '[:upper:]' '[:lower:]')
  reject "${key}-missing-ready" ".archives[0].k6_metrics |= map(select(.metric != \"xp_time_to_ready\" or .tags.resource_kind != \"$kind\"))"
  reject "${key}-missing-delete" ".archives[0].k6_metrics |= map(select(.metric != \"xp_time_to_delete\" or .tags.resource_kind != \"$kind\"))"
  reject "${key}-censored" ".archives[0].k6_metrics |= map(if .metric == \"xp_time_to_ready\" and .tags.resource_kind == \"$kind\" then .censored = true else . end)"
  reject "${key}-nonfinite" ".archives[0].k6_metrics |= map(if .metric == \"xp_time_to_delete\" and .tags.resource_kind == \"$kind\" then .finite_sample_count = 0 | .non_finite_sample_count = 1 else . end)"
  reject "${key}-missing-create" ".archives[0].k6_metrics |= map(select(.metric != \"xp_operation_duration\" or .tags.resource_kind != \"$kind\" or .tags.operation != \"create\"))"
  reject "${key}-missing-delete-operation" ".archives[0].k6_metrics |= map(select(.metric != \"xp_operation_duration\" or .tags.resource_kind != \"$kind\" or .tags.operation != \"delete\"))"
done
reject multiple-archives '.archives += [.archives[0]]'
reject policy '.policy = {"filename":"policy.yaml"}'
reject checks '.checks = [{"status":"passed"}]'
reject wrong-status '.status = "passed"'
echo 'Report verifier fixtures passed.'
