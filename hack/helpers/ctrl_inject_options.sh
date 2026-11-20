#!/usr/bin/env bash
# This script injects the provider's controller options (error backoff rate
# limiter, poll interval hook and poll-reset connector wrapper) into upjet
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
  echo "ctrl_inject_options.sh: poll hook or connector wrapper not injected into:" >&2
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

# SubaccountApiCredential needs a resource-specific adoption guard around the
# generated Terraform Framework client. Keep this customization reproducible.
target="${REPO_ROOT}/internal/controller/security/subaccountapicredential/zz_controller.go"
if [ "$(grep -c 'newCredentialAdoptionConnector(' "${target}" || true)" -eq 0 ]; then
  perl -i -0777 -pe \
    's/o\.WithExternalConnector\(\n\s*tjcontroller\.NewTerraformPluginFrameworkConnector\(/o.WithExternalConnector(\n\t\t\tnewCredentialAdoptionConnector(tjcontroller.NewTerraformPluginFrameworkConnector(/; s/(WithTerraformPluginFrameworkManagementPolicies\(o\.Features\.Enabled\(features\.EnableBetaManagementPolicies\)\)\)\)),/$1),/' \
    "${target}"
fi
if [ "$(grep -c 'newCredentialAdoptionConnector(' "${target}" || true)" -ne 1 ]; then
  echo "ctrl_inject_options.sh: failed to wrap SubaccountApiCredential's generated Framework connector" >&2
  exit 1
fi
if ! perl -0777 -ne 'my $start = index($_, "newCredentialAdoptionConnector(tjcontroller.NewTerraformPluginFrameworkConnector("); my $end = index($_, "WithTerraformPluginFrameworkManagementPolicies(o.Features.Enabled(features.EnableBetaManagementPolicies))), mgr.GetClient(), o.Features.Enabled(features.EnableBetaManagementPolicies))),"); exit($start >= 0 && $end > $start ? 0 : 1)' "${target}"; then
  perl -i -0777 -pe \
    's/(WithTerraformPluginFrameworkManagementPolicies\(o\.Features\.Enabled\(features\.EnableBetaManagementPolicies\)\))\)\)\),/$1), mgr.GetClient())),/' \
    "${target}"
fi
if ! perl -0777 -ne 'my $start = index($_, "newCredentialAdoptionConnector(tjcontroller.NewTerraformPluginFrameworkConnector("); my $end = index($_, "WithTerraformPluginFrameworkManagementPolicies(o.Features.Enabled(features.EnableBetaManagementPolicies))), mgr.GetClient(), o.Features.Enabled(features.EnableBetaManagementPolicies))),"); exit($start >= 0 && $end > $start ? 0 : 1)' "${target}"; then
  perl -i -0777 -pe \
    's/mgr\.GetClient\(\)\)\),/mgr.GetClient(), o.Features.Enabled(features.EnableBetaManagementPolicies))),/' \
    "${target}"
fi
if ! perl -0777 -ne 'my $start = index($_, "newCredentialAdoptionConnector(tjcontroller.NewTerraformPluginFrameworkConnector("); my $end = index($_, "WithTerraformPluginFrameworkManagementPolicies(o.Features.Enabled(features.EnableBetaManagementPolicies))), mgr.GetClient(), o.Features.Enabled(features.EnableBetaManagementPolicies))),"); exit($start >= 0 && $end > $start ? 0 : 1)' "${target}"; then
  echo "ctrl_inject_options.sh: failed to pass the Kubernetes client and initProvider feature to the SubaccountApiCredential state decorator" >&2
  exit 1
fi
goimports -w "${target}"
