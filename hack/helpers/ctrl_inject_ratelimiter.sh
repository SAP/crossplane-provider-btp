#!/bin/bash
# This script injects using a custom ratelimiter for backoff into upjet based controllers
# since the controller generation is part of upjet we can't do that while generating,
# so we amend this code afterwards
set -euo pipefail

find . -name "*.go" -print0 | xargs -0 perl -i -pe \
  's/o tjcontroller\.Options/o internalopts.UpjetOptions/g;
   s/internalopts\.ForControllerRuntime\(o\.Options\)/o.ForControllerRuntimeWithBackoff()/g'