#!/usr/bin/env bash
# Runs the app locally: the dev server and the page (specs/005-browser-client FR-009, spec 012 FR-020). The dev
# server accepts any test identity, and listens only on loopback. The script needs only bash 3.2 and POSIX
# tools, so a host without GNU coreutils can run the demo; the full verifier's host requirements stay its own.
#
#   bazel run //tools/dev
#
# BFF_PORT and WEB_PORT choose the loopback ports (8080 and 5173). A port that is already in use is refused
# before anything starts, by name. The first child to exit ends the demo, and both children are stopped.
set -euo pipefail
bff="$1"
web="$2"
dist="$3"
bff_port="${BFF_PORT:-8080}"
web_port="${WEB_PORT:-5173}"
for port in "$bff_port" "$web_port"; do
  case "$port" in
    ''|*[!0-9]*) echo "dev: a port must be a number, not '$port'; set BFF_PORT and WEB_PORT" >&2; exit 2 ;;
  esac
  if [ "$port" -lt 1 ] || [ "$port" -gt 65535 ]; then echo "dev: port $port is out of range; set BFF_PORT and WEB_PORT" >&2; exit 2; fi
  # A connect that succeeds means something already listens there; bash opens the socket itself, no tool needed.
  if (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
    echo "dev: port $port on 127.0.0.1 is already in use; stop that process or set BFF_PORT and WEB_PORT" >&2
    exit 2
  fi
done
origin="http://127.0.0.1:$web_port"
children=()
stop() {
  if [ ${#children[@]} -gt 0 ]; then kill "${children[@]}" 2>/dev/null || true; fi
}
trap stop EXIT
trap 'exit 0' INT TERM

"$bff" --addr="127.0.0.1:$bff_port" --allowed-origin="$origin" </dev/null &
children+=("$!")
"$web" --root "$PWD/$dist" --port "$web_port" --bff-url "http://127.0.0.1:$bff_port" </dev/null &
children+=("$!")
echo "dev: open $origin (test identity; stop with Ctrl-C)"
# Wait for the first child to exit. `wait -n` would do it in one call, but it is bash 4.3; a poll keeps the same
# semantics on bash 3.2, and the trap above stops the other child.
while :; do
  for child in "${children[@]}"; do
    if ! kill -0 "$child" 2>/dev/null; then
      wait "$child" && status=0 || status=$?
      if [ "$status" -ne 0 ]; then echo "dev: a child exited with status $status; the demo stops" >&2; fi
      exit "$status"
    fi
  done
  sleep 0.2
done
