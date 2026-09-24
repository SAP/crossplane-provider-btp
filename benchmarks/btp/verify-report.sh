#!/usr/bin/env bash
set -euo pipefail

report=${1:?usage: verify-report.sh REPORT.json}
if [[ ! -f "$report" || -L "$report" ]]; then
  echo 'benchmark report must be a regular, non-symlink JSON file' >&2
  exit 1
fi
if ! jq -e '
  def lifecycle($metric):
    [.archives[0].k6_metrics[]?
      | select(.metric == $metric and .tags.resource_kind == "Subaccount")];
  def valid_trend($metric):
    (lifecycle($metric) | length) == 1 and
    (lifecycle($metric)[0] |
      .source == "raw_k6" and
      .metric_type == "trend" and
      .sample_count >= 1 and
      .finite_sample_count == .sample_count and
      (.non_finite_sample_count // 0) == 0 and
      .censored != true and
      (.percentiles.p50 | type == "number" and isfinite));
  .schema_version == "v1" and
  .status == "not_evaluated" and
  (.policy == null) and
  ((.checks // []) | length == 0) and
  (.archives | type == "array" and length == 1) and
  valid_trend("xp_time_to_ready") and
  valid_trend("xp_time_to_delete") and
  ([.archives[0].k6_metrics[]?
    | select(.metric == "xp_operation_duration" and
      .tags.resource_kind == "Subaccount" and
      ((.tags.operation == "create" or .tags.operation == "delete") and .tags.outcome == "success"))
    | .tags.operation] | unique | sort) == ["create", "delete"]
' "$report" >/dev/null 2>&1; then
  echo 'benchmark report failed the report-only Subaccount lifecycle contract' >&2
  exit 1
fi
printf '%s\n' 'Benchmark report contract verified.'
