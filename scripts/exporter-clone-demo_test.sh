#!/usr/bin/env bash
# Safe, mocked operator-experience test for exporter-clone-demo.sh.
set -euo pipefail

command -v script >/dev/null || {
  printf 'script (util-linux) is required for this pseudo-terminal test\n' >&2
  exit 1
}

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
demo="$repo_root/scripts/exporter-clone-demo.sh"
real_git=$(command -v git)
tempdir=$(mktemp -d)
mock_bin="$tempdir/bin"
workdir="$repo_root/.work/exporter-clone-demo-script-test"
output="$tempdir/output"
# A previous interrupted mock run may have left only test-owned artifacts.
rm -rf "$workdir"
mkdir -p "$mock_bin"

cleanup() {
  if [[ -f "$workdir/provider.pid" ]]; then
    kill "$(<"$workdir/provider.pid")" 2>/dev/null || true
  fi
  rm -rf "$tempdir" "$workdir"
  rm -f /tmp/exporter-clone-demo-test-terraform
}
trap cleanup EXIT

cat >"$mock_bin/git" <<EOF
#!/usr/bin/env bash
if [[ "\$1 \$2" == "rev-parse --show-toplevel" ]]; then
  printf '%s\\n' '$repo_root'
else
  exec '$real_git' "\$@"
fi
EOF

cat >"$mock_bin/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == "build" && "$2" == "-o" ]] || exit 1
output=$3
target=$4
case "$target" in
  ./cmd/exporter)
    cat >"$output" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
[[ "${1:-}" == "login" ]] && exit 0
config=$2
raw=$(awk '$1 == "output:" {print $2}' "$config")
cat >"$raw" <<'YAML'
---
apiVersion: account.btp.sap.crossplane.io/v1alpha1
kind: Subaccount
metadata:
  name: source
spec:
  forProvider:
    subdomain: source
...
YAML
SCRIPT
    ;;
  ./scripts/exporter-clone-demo)
    cat >"$output" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
command=$1
shift
value_for() {
  local flag=$1
  shift
  while (($#)); do
    [[ "$1" == "$flag" ]] && { printf '%s' "$2"; return; }
    shift
  done
}
case "$command" in
  write-export-config)
    output=$(value_for --output "$@")
    raw=$(value_for --raw-output "$@")
    printf 'all: true\nresolve-references: true\noutput: %s\n' "$raw" >"$output"
    ;;
  login) ;;
  source-subdomain) printf 'source\n' ;;
  derive-subdomain) printf 'source-test\n' ;;
  transform)
    output=$(value_for --output "$@")
    printf '%s\n' \
      'Transformation: setting the clone subdomain so BTP creates a distinct subaccount.' \
      'Transformation: ensuring the technical user is a subaccount administrator so the provider can manage the clone.' \
      'Transformation: removing adoption external names so Crossplane creates cloned resources instead of adopting sources.' \
      'Transformation: setting full management policies so Crossplane provisions and manages the clone.' \
      'Transformation: removing stale generated-reference IDs so references resolve to cloned resources.' \
      'Transformation: normalizing clone-specific BTP values: using the target subdomain for Cloud Foundry organization names, making CIS Central entitlements enable-only, and converting service-binding names to valid Terraform identifiers.'
    cat >"$output" <<'YAML'
---
apiVersion: account.btp.sap.crossplane.io/v1alpha1
kind: Subaccount
metadata:
  name: clone
spec:
  managementPolicies:
    - "*"
  forProvider:
    subdomain: source-test
...
YAML
    ;;
esac
SCRIPT
    ;;
  ./cmd/provider)
    cat >"$output" <<'SCRIPT'
#!/usr/bin/env bash
while :; do sleep 60; done
SCRIPT
    ;;
  *) exit 1 ;;
esac
chmod +x "$output"
EOF

cat >"$mock_bin/make" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
last=${!#}
if [[ "$last" == "terraform.buildvars" ]]; then
  cat <<VARS
TERRAFORM_VERSION=1.12.3
TERRAFORM_PROVIDER_SOURCE=SAP/btp
TERRAFORM_PROVIDER_VERSION=1.23.1
TERRAFORM=/tmp/exporter-clone-demo-test-terraform
VARS
  exit 0
fi
: >/tmp/exporter-clone-demo-test-terraform
chmod +x /tmp/exporter-clone-demo-test-terraform
EOF

cat >"$mock_bin/kind" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1 $2" == "get clusters" ]]; then
  printf 'crossplane-provider-btp-exporter-demo\n'
elif [[ "$1 $2" == "export kubeconfig" ]]; then
  while (($#)); do
    [[ "$1" == "--kubeconfig" ]] && { : >"$2"; break; }
    shift
  done
fi
EOF

cat >"$mock_bin/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
arguments="$*"
if [[ "$arguments" == *"create secret generic"* ]]; then
  # Keep the mock pipeline consumer open until jq has emitted its synthetic
  # payload, avoiding a timing-dependent SIGPIPE under pipefail.
  cat >/dev/null
elif [[ "$arguments" == *"get subaccounts.account.btp.sap.crossplane.io -o json"* ]]; then
  printf '{"items":[]}\n'
elif [[ "$arguments" == *"get -f"*"-o json"* ]]; then
  cat <<'JSON'
{"kind":"List","items":[{"apiVersion":"account.btp.sap.crossplane.io/v1alpha1","kind":"Subaccount","metadata":{"namespace":"default","name":"clone"},"status":{"conditions":[{"type":"Ready","status":"True","reason":"Available","message":"created"},{"type":"Synced","status":"True","reason":"ReconcileSuccess","message":"up to date"}]}}]}
JSON
fi
EOF

cat >"$mock_bin/btp" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$mock_bin/btp"
chmod +x "$mock_bin/git" "$mock_bin/go" "$mock_bin/make" "$mock_bin/kind" "$mock_bin/kubectl"

run_demo() {
  local input=$1
  local command="env CIS_CENTRAL_BINDING='{}' BTP_TECHNICAL_USER='{\"email\":\"demo@example.invalid\",\"username\":\"demo\",\"password\":\"placeholder\"}' GLOBAL_ACCOUNT=demo CLI_SERVER_URL=https://example.invalid TECHNICAL_USER_EMAIL=demo@example.invalid '$demo' '^source$' --target-subdomain source-test --workdir '$workdir'"
  if [[ "$input" == "__EOF__" ]]; then
    PATH="$mock_bin:$PATH" script -qefc "$command" /dev/null </dev/null
  else
    PATH="$mock_bin:$PATH" script -qefc "$command" /dev/null <<<"$input"
  fi
}

if ! run_demo $'\n\n\n\n' >"$output" 2>&1; then
  cat "$output" >&2
  exit 1
fi
grep -Fq 'Exporting phase: starting' "$output"
grep -Fq 'Transforming phase: starting' "$output"
grep -Fq 'Provisioning phase: starting' "$output"
[[ $(grep -Fc 'Press Enter to continue, or Ctrl-C to cancel:' "$output") == 4 ]]
grep -Fq 'Exporter configuration YAML (complete contents; path:' "$output"
preview_line=$(grep -Fn 'Command preview:' "$output" | head -n1 | cut -d: -f1)
config_line=$(grep -Fn 'Exporter configuration YAML (complete contents; path:' "$output" | head -n1 | cut -d: -f1)
(( preview_line < config_line ))
grep -Fq 'all: true' "$output"
grep -Fq 'resolve-references: true' "$output"
! grep -Fq 'subaccount:' "$workdir/export.yaml"
grep -Fq 'Command preview:' "$output"
grep -Fq 'btp-exporter --config' "$output"
grep -Fq 'export --subaccount \^source\$' "$output"
grep -Fq 'kubectl --kubeconfig' "$output"
grep -Fq 'Raw exported YAML manifests (complete contents; path:' "$output"
! grep -Fq 'Clone-ready YAML manifests (complete contents; path:' "$output"
grep -Fq 'Raw export versus clone-ready manifest; complete colorized Git diff:' "$output"
grep -Fq 'diff --git ' "$output"
awk '
  /Raw export versus clone-ready manifest; complete colorized Git diff:/ {in_diff = 1; next}
  in_diff && /apiVersion: account.btp.sap.crossplane.io\/v1alpha1/ {complete = 1}
  END {exit !complete}
' "$output"
awk -v esc="$(printf '\033')" '
  /Raw export versus clone-ready manifest; complete colorized Git diff:/ {in_diff = 1; next}
  in_diff && index($0, esc "[") {colored = 1}
  END {exit !colored}
' "$output"
grep -Fq 'Transformation: setting the clone subdomain' "$output"
grep -Fq 'Transformation: ensuring the technical user is a subaccount administrator' "$output"
grep -Fq 'Transformation: removing adoption external names' "$output"
grep -Fq 'Transformation: setting full management policies' "$output"
grep -Fq 'Transformation: removing stale generated-reference IDs' "$output"
grep -Fq 'Transformation: normalizing clone-specific BTP values: using the target subdomain' "$output"
grep -Fq 'Provisioning phase: live managed-resource status (refreshes every 5s)' "$output"
grep -Fq 'Subaccount default/clone: Ready=' "$output"
grep -Fq 'Available: created); Synced=' "$output"
grep -Fq 'ReconcileSuccess: up to date)' "$output"
grep -Fq $'\033[32mTrue\033[0m' "$output"
grep -Fq 'Provisioning final state: all applied managed resources report Ready=True and Synced=True.' "$output"
! grep -Fq 'External BTP state:' "$output"

rm -rf "$workdir"
if run_demo __EOF__ >"$output" 2>&1; then
  printf 'expected an EOF confirmation failure\n' >&2
  exit 1
fi
grep -Fq 'an interactive terminal is required' "$output" ||
  grep -Fq 'could not read the required confirmation' "$output"

printf 'exporter-clone-demo safe script test passed\n'
