#!/usr/bin/env bash
# The verifier sources tool_paths output, and a file can resolve inside a checkout whose path holds
# shell metacharacters (specs/005-browser-client T011).
set -euo pipefail
script="$PWD/$1"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
hostile="$work/fresh [clone] (a|b)* \$x 'q"
mkdir -p "$hostile"
printf 'x\n' >"$hostile/config.yaml"
printf '#!/bin/sh\n' >"$hostile/tool"
chmod +x "$hostile/tool"
cd "$hostile"
"$script" A_FILE=config.yaml TOOL=tool >"$work/env"
set -a
# shellcheck source=/dev/null
. "$work/env"
set +a
if [[ "$A_FILE" != "$hostile/config.yaml" || "$TOOL" != "$hostile/tool" ]]; then
  echo "FAIL paths changed in sourcing: $A_FILE, $TOOL" >&2
  exit 1
fi
rc=0
"$script" A_FILE=missing >/dev/null 2>&1 || rc=$?
[[ $rc -ne 0 ]] || { echo "FAIL a missing file passed" >&2; exit 1; }
echo "tool paths passed"
