#!/usr/bin/env bash
# The repository .bazelrc is exactly the allowed set (specs/002-hermetic-skeleton review F6), so
# isolation, network, lockfile and test-filter settings cannot change unnoticed.
#
#   tools/policy/bazelrc_policy.sh <.bazelrc> <allowed lines>
set -euo pipefail
effective() { sed -e 's/[[:space:]]*$//' "$1" | grep -vE '^(#|$)' | LC_ALL=C sort; }
if ! diff <(effective "$2") <(effective "$1") >&2; then
  echo "bazelrc-policy $1:1: settings differ from tools/policy/bazelrc.allow (< allowed, > found) (rerun: tools/verify.sh)" >&2
  exit 1
fi
