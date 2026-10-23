#!/usr/bin/env bash
# This script injects custom backoff settings, the provider-wide poll
# interval hook and the poll-backoff drift-reset connector wrapper into upjet
# based controllers. Since the controller generation is part of upjet we can't
# do that while generating, so we amend this code afterwards.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# Only generated controllers. Searching REPO_ROOT would also rewrite
# zz_controller.go files in nested git worktrees (.kilo/, .scratch/, ...).
CONTROLLER_ROOT="${REPO_ROOT}/internal/controller"

find "${CONTROLLER_ROOT}" -name "zz_controller.go" -print0 | xargs -0 perl -i -0777 -pe \
  's/o tjcontroller\.Options/o internalopts.UpjetOptions/g;
   s/o\.ForControllerRuntime\(\)/o.ForControllerRuntimeWithBackoff()/g;
   s/\tif o\.PollJitter != 0 \{\n\t\topts = append\(opts, managed\.WithPollJitterHook\(o\.PollJitter\)\)\n\t\}\n/\topts = append(opts, o.PollReconcilerOptions()...)\n/g;
   s/managed\.WithExternalConnecter\(/o.WithExternalConnector(/g;
   s/(ctrl "sigs\.k8s\.io\/controller-runtime")/$1\n\tinternalopts "github.com\/sap\/crossplane-provider-btp\/internal\/controller\/options"/ unless /internalopts "github\.com\/sap\/crossplane-provider-btp\/internal\/controller\/options"/;'

# Fail if an upjet change made a substitution above stop matching; otherwise
# upjet controllers would silently lose the injected poll settings.
# `|| true`: GNU grep >= 3.5 exits 1 from -L when it lists no file, which
# would trip `set -e` in the success case.
missing="$(
  {
    find "${CONTROLLER_ROOT}" -name "zz_controller.go" -exec grep -L 'o.PollReconcilerOptions()' {} + || true
    find "${CONTROLLER_ROOT}" -name "zz_controller.go" -exec grep -L 'o.WithExternalConnector(' {} + || true
  } | sort -u
)"
if [ -n "${missing}" ]; then
  echo "ctrl_inject_backoff.sh: poll hook or connector wrapper not injected into:" >&2
  echo "${missing}" >&2
  exit 1
fi

perl -i -0777 -pe \
  's/controller\.Options/internalopts.UpjetOptions/g;
   s/"github\.com\/crossplane\/upjet\/v2\/pkg\/controller"/internalopts "github.com\/sap\/crossplane-provider-btp\/internal\/controller\/options"/g;' \
  "${REPO_ROOT}/internal/controller/zz_setup.go"

# Fix import ordering after injection
find "${CONTROLLER_ROOT}" -name "zz_controller.go" -print0 | xargs -0 goimports -w
goimports -w "${REPO_ROOT}/internal/controller/zz_setup.go"
