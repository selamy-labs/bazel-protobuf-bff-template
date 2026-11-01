#!/usr/bin/env bash
# Schema check witnesses (specs/003-notes-contract FR-005, FR-006, FR-008). A real Git repository
# holds the real schema, and each probe is one mutation of it, run through the verifier's own
# native.sh. A probe earns credit only if the unmutated schema passes, the mutation changed the
# file, and the failure names the expected rule.
#
#   tools/proto/probes_test.sh <schema> <probes.tsv> <native.sh> <NAME=path>...
set -euo pipefail
schema="$PWD/$1"
probes="$PWD/$2"
native="$PWD/$3"
shift 3
for pair in "$@"; do
  name="${pair%%=*}"
  export "$name=$PWD/${pair#*=}"
done
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

repo="$work/repo"
target="$repo/api/template/notes/v1/notes.proto"
mkdir -p "$(dirname "$target")"
cp "$schema" "$target"
chmod u+w "$target"
git -C "$repo" init -q -b main
git -C "$repo" config user.name test
git -C "$repo" config user.email test@example.com
printf 'bootstrap\n' >"$repo/api/BASELINE"
git -C "$repo" add -A
git -C "$repo" commit -q -m schema
cd "$repo"
bootstrap_output="$("$native" proto-breaking api api/BASELINE 2>&1)" ||
  fail "the bootstrap commit did not pass: $bootstrap_output"
grep -q 'compared nothing' <<<"$bootstrap_output" || fail "bootstrap did not say it compared nothing"

git rev-parse HEAD >api/BASELINE
git add -A
git commit -q -m "declare the baseline"
original="$work/original.proto"
cp "$target" "$original"

for check in format-proto lint-proto proto-breaking; do
  "$native" "$check" api api/BASELINE >"$work/out" 2>&1 || {
    cat "$work/out" >&2
    fail "control: $check rejected the unmutated schema"
  }
done

while IFS=$'\t' read -r check name expression expected; do
  [[ "$check" == \#* ]] && continue
  sed -e "$expression" "$original" >"$target"
  if cmp -s "$original" "$target"; then
    fail "$name: the mutation changed nothing"
    continue
  fi
  rc=0
  "$native" "$check" api api/BASELINE >"$work/out" 2>&1 || rc=$?
  if [[ "$expected" == - ]]; then
    [[ $rc -eq 0 ]] || { cat "$work/out" >&2; fail "$name: a compatible change was rejected"; }
  elif [[ $rc -eq 0 ]]; then
    fail "$name: passed"
  elif ! grep -qF -- "$expected" "$work/out"; then
    cat "$work/out" >&2
    fail "$name: failed without naming $expected"
  fi
  cp "$original" "$target"
done <"$probes"

# A setup failure earns nothing: buf reports a schema that does not compile as COMPILE, which no
# probe expects, and never as a compatibility rule.
sed -e 's/string body = 3/string body = /' "$original" >"$target"
rc=0
"$native" proto-breaking api api/BASELINE >"$work/out" 2>&1 || rc=$?
if [[ $rc -eq 0 ]]; then fail "a schema that does not compile passed"; fi
grep -qF '"type":"COMPILE"' "$work/out" || fail "a compile failure was not reported as COMPILE"
if grep -qE '"type":"(FIELD|RPC)_' "$work/out"; then fail "a compile failure was reported as a rule"; fi
cp "$original" "$target"

# A buf configuration beside the schema must not excuse anything.
sed -e 's/service NoteService/service Notes/' "$original" >"$target"
printf 'version: v2\nlint:\n  except: [SERVICE_SUFFIX]\n' >api/buf.yaml
rc=0
"$native" lint-proto api >"$work/out" 2>&1 || rc=$?
if [[ $rc -eq 0 ]] || ! grep -qF '"type":"SERVICE_SUFFIX"' "$work/out"; then fail "a local buf.yaml weakened lint"; fi
rm api/buf.yaml

# Nor must a comment. The directive is split so that the suppression ban does not match this file.
directive="buf:lint:"'ignore SERVICE_SUFFIX'
sed -e "s/^service NoteService/\/\/ $directive\nservice Notes/" "$original" >"$target"
rc=0
"$native" lint-proto api >"$work/out" 2>&1 || rc=$?
if [[ $rc -eq 0 ]] || ! grep -qF '"type":"SERVICE_SUFFIX"' "$work/out"; then fail "a comment excused lint"; fi
cp "$original" "$target"

# The breaking check fails closed, and a moved marker is not silently accepted.
expect_closed() { # <name> <marker text> <expected message>
  printf '%s\n' "$2" >api/BASELINE
  rc=0
  "$native" proto-breaking api api/BASELINE >"$work/out" 2>&1 || rc=$?
  if [[ $rc -ne 4 ]] || ! grep -qF -- "$3" "$work/out"; then
    cat "$work/out" >&2
    fail "$1: rc=$rc"
  fi
}
good="$(<api/BASELINE)"
expect_closed "branch name" main "must hold a full"
expect_closed "missing commit" "$(printf 'a%.0s' {1..40})" "not in this clone"
expect_closed "bootstrap with history" bootstrap "bootstrap marker is refused"
printf '%s\n' "$good" >api/BASELINE

[[ $failures -eq 0 ]] || { echo "$failures witness(es) failed" >&2; exit 1; }
echo "all schema probes passed"
