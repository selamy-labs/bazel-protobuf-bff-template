#!/usr/bin/env bash
# The real binaries (specs/004-go-bff FR-009, FR-010, FR-013). A refused configuration must exit
# nonzero before serving, and the dev server must speak the contract, stop on SIGTERM and ignore
# stdin.
#
#   bff/cmd/binaries_test.sh <bff> <bff-dev>
set -euo pipefail
bff="$PWD/$1"
dev="$PWD/$2"
work="$(mktemp -d)"
pid=""
cleanup() {
  if [[ -n "$pid" ]]; then kill "$pid" 2>/dev/null || true; fi
  rm -rf "$work"
}
trap cleanup EXIT
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

refused() { # <name> <expected message> <command...>
  local name="$1" want="$2" rc=0
  shift 2
  timeout 10 "$@" >"$work/out" 2>"$work/err" </dev/null || rc=$?
  [[ $rc -eq 1 ]] || fail "$name: exit $rc"
  grep -qF -- "$want" "$work/err" || { cat "$work/err" >&2; fail "$name: message"; }
  if [[ -s "$work/out" ]]; then fail "$name: printed to stdout, so it may have reported ready"; fi
}

refused "shipped production binary" "no verifier is configured" env -u BFF_MODE "$bff"
refused "production with a dev mode" "only in production" "$bff" --mode=dev
refused "production with dev mode from the environment" "only in production" env BFF_MODE=dev "$bff"
refused "production with test identity" "refuses identity" "$bff" --identity=test-identity
refused "dev on every interface" "loopback" "$dev" --addr=0.0.0.0:0
refused "dev on a host name" "loopback" "$dev" --addr=localhost:0
refused "dev with a wildcard origin" "exact" "$dev" --addr=127.0.0.1:0 --allowed-origin='*'

# stdin stays open and unread: a server that waited on it would never report ready.
mkfifo "$work/stdin"
sleep 60 >"$work/stdin" &
holder=$!
"$dev" --addr=127.0.0.1:0 --allowed-origin=http://127.0.0.1:5173 \
  <"$work/stdin" >"$work/stdout" 2>"$work/stderr" &
pid=$!
for _ in $(seq 1 500); do
  if [[ -s "$work/stdout" ]]; then break; fi
  sleep 0.01
done
kill "$holder" 2>/dev/null || true
[[ "$(wc -l <"$work/stdout")" -eq 1 ]] || fail "want one ready event, got: $(cat "$work/stdout")"
grep -q '"event":"ready"' "$work/stdout" || fail "no ready event"
grep -q '"identity":"test-identity"' "$work/stdout" || fail "the ready event hides test identity"
grep -q 'TEST IDENTITY' "$work/stderr" || fail "the dev binary did not say what it is"
addr="$(sed -E 's/.*"addr":"([^"]+)".*/\1/' "$work/stdout")"

call() { # <authorization header value or empty>
  local header=()
  if [[ -n "$1" ]]; then header=(-H "Authorization: $1"); fi
  curl -sS --max-time 5 -o "$work/body" -w '%{http_code}' -X POST \
    -H 'Content-Type: application/json' "${header[@]}" \
    -d '{"note":{"title":"a title"}}' "http://$addr/template.notes.v1.NoteService/CreateNote"
}
status="$(call 'Bearer test-identity.ana')"
[[ "$status" == 200 ]] || fail "create: $status $(cat "$work/body")"
grep -q '"name":"notes/' "$work/body" || fail "create body: $(cat "$work/body")"
status="$(call '')"
[[ "$status" == 401 ]] || fail "no credential: $status"

kill -TERM "$pid"
rc=0
wait "$pid" || rc=$?
pid=""
[[ $rc -eq 0 ]] || fail "SIGTERM: exit $rc"
if curl -sS --max-time 2 "http://$addr/" >/dev/null 2>&1; then fail "still serving after SIGTERM"; fi

[[ $failures -eq 0 ]] || { echo "$failures witness(es) failed" >&2; exit 1; }
echo "binary witnesses passed"
