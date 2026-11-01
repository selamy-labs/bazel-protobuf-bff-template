#!/usr/bin/env bash
# The local demo's launcher (spec 012 FR-020 to FR-022): the page can reach its server, the profile is the one
# default site with named refusals, a busy port is refused by name, notes persist across a restart and a reset
# clears them, the first child's exit ends the demo, and SIGTERM stops both children.
#
#   tools/dev/dev_test.sh <dev runner> <arguments for it>...
set -euo pipefail
runner="$1"
shift
work="$(mktemp -d)"
pid=""
trap 'if [[ -n "$pid" ]]; then kill "$pid" 2>/dev/null || true; fi; rm -rf "$work"' EXIT
export BFF_PORT="$((20000 + RANDOM % 10000))" WEB_PORT="$((30000 + RANDOM % 10000))" BFF_DATA_DIR="$work/notes" BFF_STORE=local
web="http://127.0.0.1:$WEB_PORT"
bff="http://127.0.0.1:$BFF_PORT"
start() { # start <output file> [launcher arguments...]: $pid is the launcher, started in its own session
  local out="$1"
  shift
  setsid "$runner" "$@" >"$out" 2>&1 </dev/null &
  pid=$!
  for _ in $(seq 1 500); do
    if curl -sf --max-time 1 "$web/config.json" -o "$work/config" && curl -s --max-time 1 "$bff/" -o /dev/null; then return 0; fi
    if ! kill -0 "$pid" 2>/dev/null; then echo "FAIL the launcher exited early: $(cat "$out")" >&2; exit 1; fi
    sleep 0.02
  done
  echo "FAIL the stack did not come up: $(cat "$out")" >&2
  exit 1
}
stop() { # stop: SIGTERM to the launcher, then both ports must close
  kill -TERM "$pid"
  wait "$pid" || true
  pid=""
  for _ in $(seq 1 200); do
    if ! curl -s --max-time 1 "$web/" -o /dev/null && ! curl -s --max-time 1 "$bff/" -o /dev/null; then return 0; fi
    sleep 0.02
  done
  echo "FAIL a child survived SIGTERM" >&2
  exit 1
}
rpc() { # rpc <subject> <method> <json>: a Connect call as the test identity
  curl -s --max-time 2 -H 'Content-Type: application/json' -H "Authorization: Bearer test-identity.$1" -H "Origin: $web" \
    -d "$3" "$bff/template.notes.v1.NoteService/$2"
}

start "$work/out" "$@"
grep -qF "\"bffUrl\":\"$bff\"" "$work/config" || { echo "FAIL config: $(cat "$work/config")" >&2; exit 1; }
curl -sf "$web/" | grep -q 'id="root"' || { echo "FAIL the page was not served" >&2; exit 1; }
curl -sf "$web/main.js" -o /dev/null || { echo "FAIL the bundle was not served" >&2; exit 1; }
headers="$(curl -s -D - -o /dev/null -X OPTIONS -H "Origin: $web" -H 'Access-Control-Request-Method: POST' \
  "$bff/template.notes.v1.NoteService/GetNote")"
grep -qi "access-control-allow-origin: $web" <<<"$headers" || { echo "FAIL CORS: $headers" >&2; exit 1; }
grep -q 'test identity' "$work/out" || { echo "FAIL the runner hid test identity" >&2; exit 1; }
grep -q "bff_port=\"$BFF_PORT\" # from BFF_PORT" "$work/out" || { echo "FAIL the effective port and its source were not printed: $(cat "$work/out")" >&2; exit 1; }
grep -q 'store="local" # from BFF_STORE' "$work/out" || { echo "FAIL the store and its source were not printed" >&2; exit 1; }
grep -q "notes persist in $work/notes" "$work/out" || { echo "FAIL the persistence statement was not printed" >&2; exit 1; }
# A busy port is refused before anything starts, by name: a second runner on the same ports.
rc=0; "$runner" "$@" >"$work/busy" 2>&1 </dev/null || rc=$?
if [[ "$rc" != 2 ]] || ! grep -q "port $BFF_PORT on 127.0.0.1 is already in use" "$work/busy"; then echo "FAIL a busy port was not refused by name: exit $rc: $(cat "$work/busy")" >&2; exit 1; fi
rc=0; BFF_PORT=notaport "$runner" "$@" >"$work/bad" 2>&1 </dev/null || rc=$?
if [[ "$rc" != 2 ]] || ! grep -q "BFF_PORT" "$work/bad"; then echo "FAIL a bad port value was not refused by name: exit $rc: $(cat "$work/bad")" >&2; exit 1; fi
rc=0; BFF_STORE=cloud "$runner" "$@" >"$work/bad" 2>&1 </dev/null || rc=$?
if [[ "$rc" != 2 ]] || ! grep -q "store" "$work/bad"; then echo "FAIL an unknown store was not refused by name: exit $rc" >&2; exit 1; fi
rc=0; "$runner" "$@" --something >"$work/bad" 2>&1 </dev/null || rc=$?
if [[ "$rc" != 2 ]] || ! grep -q "unknown argument" "$work/bad"; then echo "FAIL an unknown argument was not refused: exit $rc" >&2; exit 1; fi
# A note persists across a restart, and a reset clears it (spec 012 FR-022).
created="$(rpc ana CreateNote '{"note":{"title":"kept","body":"across a restart"}}')"
name="$(sed -nE 's/.*"name":"(notes\/[a-z0-9-]+)".*/\1/p' <<<"$created")"
[[ -n "$name" ]] || { echo "FAIL create: $created" >&2; exit 1; }
stop
start "$work/out2" "$@"
rpc ana GetNote "{\"name\":\"$name\"}" | grep -q '"body":"across a restart"' || { echo "FAIL the note did not survive a restart" >&2; exit 1; }
rpc bob GetNote "{\"name\":\"$name\"}" | grep -q 'not_found' || { echo "FAIL another owner read the note after a restart" >&2; exit 1; }
stop
start "$work/out3" "$@" --reset
grep -q "reset: removed $work/notes" "$work/out3" || { echo "FAIL the reset was not reported" >&2; exit 1; }
rpc ana GetNote "{\"name\":\"$name\"}" | grep -q 'not_found' || { echo "FAIL the note survived a reset" >&2; exit 1; }
stop
# The first child to exit ends the demo and the other child is stopped (the launcher does not need `wait -n`).
start "$work/out4" "$@"
backend="$(pgrep -P "$pid" -f -- '--addr=127.0.0.1' | head -1)"
[[ -n "$backend" ]] || { echo "FAIL the backend child was not found" >&2; exit 1; }
kill -KILL "$backend"
for _ in $(seq 1 200); do if ! kill -0 "$pid" 2>/dev/null; then break; fi; sleep 0.05; done
if kill -0 "$pid" 2>/dev/null; then echo "FAIL the launcher outlived its first dead child" >&2; exit 1; fi
rc=0; wait "$pid" || rc=$?
pid=""
[[ "$rc" -ne 0 ]] || { echo "FAIL the launcher exited 0 after a child died" >&2; exit 1; }
grep -q "a child exited with status" "$work/out4" || { echo "FAIL the child's exit was not reported" >&2; exit 1; }
for _ in $(seq 1 200); do
  if ! curl -s --max-time 1 "$web/" -o /dev/null; then echo "dev runner passed"; exit 0; fi
  sleep 0.02
done
echo "FAIL the page child survived the backend's death" >&2
exit 1
