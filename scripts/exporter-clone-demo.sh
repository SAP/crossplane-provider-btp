#!/usr/bin/env bash
# Export one BTP subaccount, prepare a clone manifest, and run it against a
# local kind cluster. Credentials must already be present in the environment.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/exporter-clone-demo.sh SOURCE_SUBACCOUNT_SELECTOR [options]

Export all supported resources from one source subaccount, create a clone-ready
manifest, and apply it with a locally running provider against a named kind
cluster. Raw exports, manifests, kubeconfig, provider PID, and logs are kept
beneath .work/; the script never removes them or BTP resources.

Options:
  --target-subdomain SUBDOMAIN  Use this subdomain instead of deriving one from
                                the source subdomain and BUILD_ID.
  --workdir PATH                Directory for artifacts. It must be inside this
                                repository's .work/ directory.
  --cluster-name NAME           Kind cluster to create or reuse.
                                Default: crossplane-provider-btp-exporter-demo
  --timeout DURATION            Timeout for CRD establishment and managed
                                resource readiness. Default: 30m.
  --dry-run                     Validate arguments and required environment
                                variable names without accessing BTP or Kubernetes.
                                Requires --target-subdomain.
  -h, --help                    Show this help.

Required environment variables:
  CIS_CENTRAL_BINDING, BTP_TECHNICAL_USER, GLOBAL_ACCOUNT, CLI_SERVER_URL, and
  TECHNICAL_USER_EMAIL. BUILD_ID is also required unless --target-subdomain is
  supplied.

The demo environment also declares SECOND_DIRECTORY_ADMIN_EMAIL, IDP_URL, and
BUILD_REGISTRY for source-landscape preparation. This script never reads
e2e.env; it uses only environment variables inherited by this process.
EOF
}

current_phase=""

fail() {
  if [[ -n "$current_phase" ]]; then
    printf 'error: %s phase failed: %s\n' "$current_phase" "$*" >&2
  else
    printf 'error: %s\n' "$*" >&2
  fi
  exit 1
}

phase_start() {
  current_phase=$1
  printf '\n================================================================\n'
  printf ' %s phase: starting\n' "$current_phase"
  printf '================================================================\n'
}

phase_complete() {
  printf '\n%s phase: completed.\n' "$current_phase"
}

print_command() {
  local argument
  printf 'Command preview: '
  for argument in "$@"; do
    printf '%q ' "$argument"
  done
  printf '\n'
}

show_manifest() {
  local label=$1
  local path=$2
  printf '\n%s (complete contents; path: %s)\n' "$label" "$path"
  printf '%s\n' '----------------------------------------------------------------'
  cat "$path"
  printf '%s\n' '----------------------------------------------------------------'
}

show_manifest_diff() {
  local raw=$1
  local transformed=$2
  local diff_status=0
  local raw_lines transformed_lines context_lines
  raw_lines=$(wc -l <"$raw")
  transformed_lines=$(wc -l <"$transformed")
  if (( raw_lines > transformed_lines )); then
    context_lines=$raw_lines
  else
    context_lines=$transformed_lines
  fi
  context_lines=$((context_lines + 1))

  printf '\nRaw export versus clone-ready manifest; complete colorized Git diff:\n'
  printf 'Raw: %s\nClone-ready: %s\n' "$raw" "$transformed"
  printf '%s\n' '----------------------------------------------------------------'
  # git diff uses familiar unified hunks with line-level color and returns 1
  # when files differ, which is the expected successful result here.
  git --no-pager diff --no-index --no-ext-diff --minimal --color=always --unified="$context_lines" -- "$raw" "$transformed" || diff_status=$?
  if (( diff_status > 1 )); then
    fail "could not render the raw-to-clone-ready Git diff"
  fi
  printf '%s\n' '----------------------------------------------------------------'
}

confirm_phase() {
  local message=$1
  [[ -t 0 && -t 1 ]] || fail "an interactive terminal is required; rerun from a terminal to continue"
  printf '\n%s\n' "$message"
  printf 'Press Enter to continue, or Ctrl-C to cancel: '
  if ! IFS= read -r; then
    fail "could not read the required confirmation"
  fi
}

expected_env_vars=(
  CIS_CENTRAL_BINDING
  BTP_TECHNICAL_USER
  BUILD_ID
  GLOBAL_ACCOUNT
  CLI_SERVER_URL
  SECOND_DIRECTORY_ADMIN_EMAIL
  TECHNICAL_USER_EMAIL
  IDP_URL
  BUILD_REGISTRY
)
source_selector=""
target_subdomain=""
workdir=""
cluster_name="crossplane-provider-btp-exporter-demo"
timeout="30m"
dry_run=false

while (($#)); do
  case "$1" in
    --target-subdomain)
      (($# >= 2)) || fail "--target-subdomain requires a value"
      target_subdomain=$2
      shift 2
      ;;
    --workdir)
      (($# >= 2)) || fail "--workdir requires a value"
      workdir=$2
      shift 2
      ;;
    --cluster-name)
      (($# >= 2)) || fail "--cluster-name requires a value"
      cluster_name=$2
      shift 2
      ;;
    --timeout)
      (($# >= 2)) || fail "--timeout requires a value"
      timeout=$2
      shift 2
      ;;
    --dry-run)
      dry_run=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --*)
      fail "unknown option: $1"
      ;;
    *)
      [[ -z "$source_selector" ]] || fail "only one source subaccount selector is allowed"
      source_selector=$1
      shift
      ;;
  esac
done

[[ -n "$source_selector" ]] || {
  usage >&2
  exit 2
}
[[ -n "$cluster_name" ]] || fail "--cluster-name must not be empty"

for variable in "${expected_env_vars[@]}"; do
  case "$variable" in
    CIS_CENTRAL_BINDING|BTP_TECHNICAL_USER|GLOBAL_ACCOUNT|CLI_SERVER_URL|TECHNICAL_USER_EMAIL)
      [[ -n "${!variable:-}" ]] || fail "required environment variable is not set: $variable"
      ;;
    BUILD_ID)
      if [[ -z "$target_subdomain" ]]; then
        [[ -n "${!variable:-}" ]] || fail "required environment variable is not set: BUILD_ID (or supply --target-subdomain)"
      fi
      ;;
  esac
done

validate_target_subdomain() {
  [[ "$1" =~ ^[a-z0-9]([a-z0-9-]{1,61}[a-z0-9])$ ]] ||
    fail "invalid target subdomain; expected 3-63 lowercase letters, digits, or hyphens, beginning and ending with a letter or digit"
}

if [[ -n "$target_subdomain" ]]; then
  validate_target_subdomain "$target_subdomain"
fi

if [[ "$dry_run" == true ]]; then
  [[ -n "$target_subdomain" ]] || fail "--dry-run requires --target-subdomain because the source subdomain is available only after export"
  printf '%s\n' "Dry run passed: arguments and required environment-variable names are valid."
  printf '%s\n' "No BTP, kind, kubectl, build, or filesystem operations were performed."
  exit 0
fi

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required tool is not available on PATH: $1"
}

# The repository's Go-backed transform-manifests helper is the selected YAML
# transformation tool; jq validates and shapes JSON without persisting secrets.
for tool in go btp kind kubectl jq; do
  require_command "$tool"
done

repo_root=$(git rev-parse --show-toplevel) || fail "run from inside the repository"
# Ask Make for the effective E2E settings. In particular, TERRAFORM is the
# OpenTofu executable built by the provider's Makefile and must be the binary
# placed on PATH: Upjet always invokes the literal command "terraform".
make_buildvar() {
  local variable=$1
  make -s -C "$repo_root" terraform.buildvars |
    awk -F= -v variable="$variable" '$1 == variable && value == "" {value = substr($0, length(variable) + 2)} END {print value}'
}
terraform_version=$(make_buildvar TERRAFORM_VERSION)
terraform_provider_source=$(make_buildvar TERRAFORM_PROVIDER_SOURCE)
terraform_provider_version=$(make_buildvar TERRAFORM_PROVIDER_VERSION)
terraform_binary=$(make_buildvar TERRAFORM)
for variable in terraform_version terraform_provider_source terraform_provider_version terraform_binary; do
  [[ -n "${!variable}" ]] || fail "could not determine ${variable#terraform_} from $repo_root/Makefile"
done
work_root="$repo_root/.work"
mkdir -p "$work_root"

if [[ -z "$workdir" ]]; then
  workdir=$(mktemp -d "$work_root/exporter-clone-demo.XXXXXX")
else
  case "$workdir" in
    /*) ;;
    *) workdir="$repo_root/$workdir" ;;
  esac
  workdir=$(realpath -m "$workdir")
  case "$workdir" in
    "$work_root"/*) ;;
    *) fail "--workdir must be inside $work_root" ;;
  esac
  if [[ -e "$workdir" ]] && [[ -n "$(find "$workdir" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
    fail "--workdir must be empty so existing exports are not overwritten: $workdir"
  fi
  mkdir -p "$workdir"
fi

exporter="$workdir/btp-exporter"
transformer="$workdir/transform-manifests"
provider="$workdir/provider"
config="$workdir/export.yaml"
raw_export="$workdir/raw-export.yaml"
clone_export="$workdir/clone-ready.yaml"
kubeconfig="$workdir/kubeconfig"
provider_log="$workdir/provider.log"
provider_pid_file="$workdir/provider.pid"
context="kind-$cluster_name"
provider_pid=""

kubectl_context() {
  kubectl --kubeconfig "$kubeconfig" --context "$context" "$@"
}

report_failure() {
  local exit_code=$?
  trap - ERR
  printf '\n%s phase failed (exit %d). No cleanup was performed.\n' "${current_phase:-Setup}" "$exit_code" >&2
  if [[ -f "$kubeconfig" ]]; then
    printf 'Non-sensitive resource status:\n' >&2
    kubectl_context get -f "$clone_export" -o wide 2>&1 || true
    printf 'Recent Kubernetes events:\n' >&2
    kubectl_context get events --all-namespaces --sort-by=.lastTimestamp 2>&1 | tail -n 80 || true
  fi
  if [[ -n "$provider_pid" ]] && ! kill -0 "$provider_pid" 2>/dev/null; then
    printf 'The provider process exited during setup.\n' >&2
  fi
  printf 'Provider log: %s\nProvider PID file: %s\nKubeconfig: %s\n' \
    "$provider_log" "$provider_pid_file" "$kubeconfig" >&2
  exit "$exit_code"
}
trap report_failure ERR

resource_statuses() {
  kubectl_context get -f "$clone_export" -o json | jq -r '
    def items: if .kind == "List" then .items else [.] end;
    def condition($type): ([((.status.conditions? // [])[] | select(.type == $type))] | last // {});
    def text: tostring | gsub("[\\t\\r\\n]"; " ");
    items[]
    | select((.apiVersion // "") | contains("btp.sap.crossplane.io/"))
    | . as $resource
    | (condition("Ready")) as $ready
    | (condition("Synced")) as $synced
    | [
        $resource.apiVersion,
        $resource.kind,
        ($resource.metadata.namespace // "default"),
        $resource.metadata.name,
        ($ready.status // "Unknown"),
        (($ready.reason // "") + ": " + ($ready.message // "") | text),
        ($synced.status // "Unknown"),
        (($synced.reason // "") + ": " + ($synced.message // "") | text)
      ]
    | @tsv
  '
}

condition_color() {
  case "$1" in
    True) printf '\033[32m' ;;   # green
    False) printf '\033[31m' ;;  # red
    *) printf '\033[33m' ;;      # yellow: unknown, pending, or absent
  esac
}

color_condition() {
  # The live flow requires an interactive terminal for its confirmations, so
  # color is always available when this dashboard can be reached.
  printf '%b%s\033[0m' "$(condition_color "$1")" "$1"
}

report_resource_statuses() {
  local states api_version kind namespace name ready ready_detail synced synced_detail ready_display synced_display
  if ! states=$(resource_statuses); then
    fail "could not retrieve managed-resource status from the kind cluster"
  fi
  [[ -n "$states" ]] || fail "the clone-ready manifest did not resolve to any BTP managed resources"

  # Redraw a fixed status view instead of appending an unreadable stream of
  # polls. This deliberately behaves like watch without adding another tool.
  printf '\033[2J\033[H'
  printf '================================================================\n'
  printf ' Provisioning phase: live managed-resource status (refreshes every 5s)\n'
  printf ' Elapsed: %ss / timeout: %s\n' "$((SECONDS - provisioning_started))" "$timeout"
  printf '================================================================\n'
  all_resources_ready=true
  while IFS=$'\t' read -r api_version kind namespace name ready ready_detail synced synced_detail; do
    ready_display=$(color_condition "$ready")
    synced_display=$(color_condition "$synced")
    printf '  %s %s/%s: Ready=%s (%s); Synced=%s (%s)\n' \
      "$kind" "$namespace" "$name" "$ready_display" "$ready_detail" "$synced_display" "$synced_detail"
    [[ "$ready" == "True" && "$synced" == "True" ]] || all_resources_ready=false
  done <<<"$states"
}

poll_managed_resources() {
  local elapsed
  provisioning_started=$SECONDS
  while :; do
    report_resource_statuses
    if [[ "$all_resources_ready" == true ]]; then
      printf '\nProvisioning final state: all applied managed resources report Ready=True and Synced=True.\n'
      return 0
    fi
    elapsed=$((SECONDS - provisioning_started))
    if (( elapsed >= timeout_seconds )); then
      printf 'Provisioning timed out after %s. Final managed-resource diagnostics follow:\n' "$timeout" >&2
      report_resource_statuses >&2 || true
      kubectl_context get -f "$clone_export" -o yaml >&2 || true
      return 1
    fi
    sleep 5
  done
}

[[ "$timeout" =~ ^[1-9][0-9]*[smh]$ ]] || fail "--timeout must be a positive whole-number s, m, or h duration"
case "$timeout" in
  *s) timeout_seconds=${timeout%s} ;;
  *m) timeout_seconds=$(( ${timeout%m} * 60 )) ;;
  *h) timeout_seconds=$(( ${timeout%h} * 3600 )) ;;
esac

phase_start "Exporting"
# Build all executable artifacts in .work/. The committed transformer source has
# no credentials and the exporter configuration contains only selection details.
go build -o "$exporter" ./cmd/exporter
go build -o "$transformer" ./scripts/exporter-clone-demo
go build -o "$provider" ./cmd/provider

"$transformer" write-export-config \
  --output "$config" \
  --raw-output "$raw_export"

# The helper parses BTP_TECHNICAL_USER only in memory and passes exporter login
# settings through its environment. No credential is written to disk or supplied
# as a shell command-line argument.
"$transformer" login --exporter "$exporter"
print_command "$exporter" --config "$config" export --subaccount "$source_selector"
show_manifest "Exporter configuration YAML" "$config"
confirm_phase "The exporter command above will access BTP and export the selected subaccount."
"$exporter" --config "$config" export --subaccount "$source_selector"
show_manifest "Raw exported YAML manifests" "$raw_export"
phase_complete
confirm_phase "Exporting is complete. Review the raw manifest above before beginning Transforming."

phase_start "Transforming"
source_subdomain=$("$transformer" source-subdomain --input "$raw_export")
if [[ -z "$target_subdomain" ]]; then
  target_subdomain=$("$transformer" derive-subdomain \
    --source-subdomain "$source_subdomain" \
    --build-id "$BUILD_ID")
fi

"$transformer" transform \
  --input "$raw_export" \
  --output "$clone_export" \
  --target-subdomain "$target_subdomain" \
  --technical-user-email "$TECHNICAL_USER_EMAIL"
confirm_phase "Transformation is complete. Press Enter to display the raw-to-clone-ready diff."
show_manifest_diff "$raw_export" "$clone_export"
phase_complete
confirm_phase "Transforming is complete. Review the diff above before beginning Provisioning."

phase_start "Provisioning"
if kind get clusters | grep -Fxq "$cluster_name"; then
  printf 'Reusing kind cluster: %s\n' "$cluster_name"
else
  printf 'Creating kind cluster: %s\n' "$cluster_name"
  kind create cluster --name "$cluster_name"
fi
kind export kubeconfig --name "$cluster_name" --kubeconfig "$kubeconfig"
kubectl --kubeconfig "$kubeconfig" config use-context "$context" >/dev/null
kubectl_context cluster-info

kubectl_context apply -R -f "$repo_root/package/crds"
kubectl_context wait --for=condition=Established --timeout="$timeout" -R -f "$repo_root/package/crds"

create_secret_from_stdin() {
  local name=$1
  local key=$2
  kubectl_context create secret generic "$name" \
    --from-file="$key=/dev/stdin" \
    --dry-run=client -o yaml | kubectl_context apply -f -
}

# The two secret payloads flow directly from inherited variables into kubectl.
# They are never rendered to a file, command line, or diagnostic output.
printf '%s' "$CIS_CENTRAL_BINDING" | jq -ce '
  if type != "object" then error("CIS_CENTRAL_BINDING must be JSON")
  elif has("credentials") then .credentials
  else .
  end | if type == "object" then . else error("CIS credentials must be a JSON object") end
' | create_secret_from_stdin cis-provider-secret data
printf '%s' "$BTP_TECHNICAL_USER" | jq -ce '
  if type != "object" then error("BTP_TECHNICAL_USER must be JSON")
  elif (.email | type == "string" and length > 0)
   and (.username | type == "string" and length > 0)
   and (.password | type == "string" and length > 0) then .
  else error("BTP_TECHNICAL_USER must contain non-empty email, username, and password fields")
  end
' | create_secret_from_stdin sa-provider-secret credentials

# ProviderConfig contains secret references and connection settings only. It is
# generated in-memory so no configuration artifact containing environment data
# is written to the workspace.
jq -n \
  --arg global_account "$GLOBAL_ACCOUNT" \
  --arg cli_server_url "$CLI_SERVER_URL" \
  '{apiVersion: "btp.sap.crossplane.io/v1alpha1", kind: "ProviderConfig", metadata: {name: "default"}, spec: {globalAccount: $global_account, cliServerUrl: $cli_server_url, cisCredentials: {source: "Secret", secretRef: {namespace: "default", name: "cis-provider-secret", key: "data"}}, serviceAccountSecret: {source: "Secret", secretRef: {namespace: "default", name: "sa-provider-secret", key: "credentials"}}}}' |
  kubectl_context apply -f -

# Upjet runs the literal command "terraform", ignoring the version held in
# its setup metadata. Build the Makefile-selected OpenTofu binary, then expose
# it under that exact command name only for the local provider process.
make -C "$repo_root" "$terraform_binary" || fail "could not build OpenTofu binary: $terraform_binary"
[[ -x "$terraform_binary" ]] || fail "Makefile-selected OpenTofu binary is not executable: $terraform_binary"
terraform_bin_dir="$workdir/terraform-bin"
mkdir -p "$terraform_bin_dir"
ln -s "$terraform_binary" "$terraform_bin_dir/terraform"

# Keep the controller's kubeconfig and Terraform executable isolated from the
# user's active context and host PATH.
printf 'Using E2E OpenTofu version: %s; BTP provider: %s@%s\n' \
  "$terraform_version" "$terraform_provider_source" "$terraform_provider_version"
(
  cd "$repo_root"
  exec env KUBECONFIG="$kubeconfig" PATH="$terraform_bin_dir:$PATH" "$provider" --debug \
    --terraform-version="$terraform_version" \
    --terraform-provider-source="$terraform_provider_source" \
    --terraform-provider-version="$terraform_provider_version"
) >"$provider_log" 2>&1 &
provider_pid=$!
printf '%s\n' "$provider_pid" >"$provider_pid_file"
sleep 2
kill -0 "$provider_pid" 2>/dev/null || fail "local provider exited during setup"

# A previous run is intentionally retained. Refuse to reuse its target
# subdomain in this cluster so each subsequent clone needs a new BUILD_ID or an
# explicitly different --target-subdomain.
if kubectl_context get subaccounts.account.btp.sap.crossplane.io -o json |
  jq -e --arg subdomain "$target_subdomain" \
    'any(.items[]; .spec.forProvider.subdomain == $subdomain)' >/dev/null; then
  fail "target subdomain already exists in this cluster: $target_subdomain; supply a new BUILD_ID or --target-subdomain"
fi

kill -0 "$provider_pid" 2>/dev/null || fail "local provider exited before applying clone-ready manifests"
print_command kubectl --kubeconfig "$kubeconfig" --context "$context" apply -f "$clone_export"
kubectl_context apply -f "$clone_export"
poll_managed_resources
kill -0 "$provider_pid" 2>/dev/null || fail "local provider exited while resources were becoming ready"
phase_complete

printf '\nDemo completed successfully. No cleanup was performed.\n'
printf 'Raw export: %s\nClone-ready manifest: %s\nTarget subdomain: %s\n' \
  "$raw_export" "$clone_export" "$target_subdomain"
printf 'Kind cluster: %s\nKubeconfig: %s\nProvider PID: %s\nProvider log: %s\n' \
  "$cluster_name" "$kubeconfig" "$provider_pid_file" "$provider_log"
printf 'Inspect resources: kubectl --kubeconfig %q --context %q get -f %q -o wide\n' \
  "$kubeconfig" "$context" "$clone_export"
printf 'Inspect events: kubectl --kubeconfig %q --context %q get events --all-namespaces --sort-by=.lastTimestamp\n' \
  "$kubeconfig" "$context"
printf 'Stop the provider when finished: kill %q\n' "$provider_pid"
