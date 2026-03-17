#!/bin/bash
# This script injects using a custom ratelimiter for backoff into upjet based controllers
# since the controller generation is part of upjet we can't do that while generating,
# so we amend this code afterwards
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

find "${REPO_ROOT}" -name "zz_controller.go" -print0 | xargs -0 perl -i -0777 -pe \
  's/o tjcontroller\.Options/o internalopts.UpjetOptions/g;
   s/o\.ForControllerRuntime\(\)/o.ForControllerRuntimeWithBackoff()/g;
   s/(ctrl "sigs\.k8s\.io\/controller-runtime")/$1\n\tinternalopts "github.com\/sap\/crossplane-provider-btp\/internal\/controller\/options"/ unless /internalopts "github\.com\/sap\/crossplane-provider-btp\/internal\/controller\/options"/;'


perl -i -0777 -pe \
  's/controller\.Options/internalopts.UpjetOptions/g;
   s/"github\.com\/crossplane\/upjet\/pkg\/controller"/internalopts "github.com\/sap\/crossplane-provider-btp\/internal\/controller\/options"/g;' \
  "${REPO_ROOT}/internal/controller/zz_setup.go"
