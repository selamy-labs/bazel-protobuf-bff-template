#!/usr/bin/env bash
# Runs the app locally: the dev server and the page (specs/005-browser-client FR-009). The dev
# server accepts any test identity, and listens only on loopback.
#
#   bazel run //tools/dev
set -euo pipefail
bff="$1"
web="$2"
dist="$3"
bff_port="${BFF_PORT:-8080}"
web_port="${WEB_PORT:-5173}"
origin="http://127.0.0.1:$web_port"
children=()
stop() {
  if [[ ${#children[@]} -gt 0 ]]; then kill "${children[@]}" 2>/dev/null || true; fi
}
trap stop EXIT
trap 'exit 0' INT TERM

"$bff" --addr="127.0.0.1:$bff_port" --allowed-origin="$origin" </dev/null &
children+=("$!")
"$web" --root "$PWD/$dist" --port "$web_port" --bff-url "http://127.0.0.1:$bff_port" </dev/null &
children+=("$!")
echo "dev: open $origin (test identity; stop with Ctrl-C)"
wait -n "${children[@]}"
