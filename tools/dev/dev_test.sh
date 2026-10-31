#!/usr/bin/env bash
# The dev runner serves a page that can reach its server, and stops both on SIGTERM.
#
#   tools/dev/dev_test.sh <dev runner> <arguments for it>...
set -euo pipefail
runner="$1"
shift
work="$(mktemp -d)"
pid=""
trap 'if [[ -n "$pid" ]]; then kill "$pid" 2>/dev/null || true; fi; rm -rf "$work"' EXIT
export BFF_PORT="$((20000 + RANDOM % 10000))" WEB_PORT="$((30000 + RANDOM % 10000))"
setsid "$runner" "$@" >"$work/out" 2>&1 </dev/null &
pid=$!
web="http://127.0.0.1:$WEB_PORT"
bff="http://127.0.0.1:$BFF_PORT"
for _ in $(seq 1 500); do
  if curl -sf --max-time 1 "$web/config.json" -o "$work/config" && curl -s --max-time 1 "$bff/" -o /dev/null; then break; fi
  sleep 0.02
done
grep -qF "\"bffUrl\":\"$bff\"" "$work/config" || { echo "FAIL config: $(cat "$work/config")" >&2; exit 1; }
curl -sf "$web/" | grep -q 'id="root"' || { echo "FAIL the page was not served" >&2; exit 1; }
curl -sf "$web/main.js" -o /dev/null || { echo "FAIL the bundle was not served" >&2; exit 1; }
headers="$(curl -s -D - -o /dev/null -X OPTIONS -H "Origin: $web" -H 'Access-Control-Request-Method: POST' \
  "$bff/template.notes.v1.NoteService/GetNote")"
grep -qi "access-control-allow-origin: $web" <<<"$headers" || { echo "FAIL CORS: $headers" >&2; exit 1; }
grep -q 'test identity' "$work/out" || { echo "FAIL the runner hid test identity" >&2; exit 1; }
kill -TERM "$pid"
wait "$pid" || true
pid=""
for _ in $(seq 1 200); do
  if ! curl -s --max-time 1 "$web/" -o /dev/null && ! curl -s --max-time 1 "$bff/" -o /dev/null; then
    echo "dev runner passed"
    exit 0
  fi
  sleep 0.02
done
echo "FAIL a child survived SIGTERM" >&2
exit 1
