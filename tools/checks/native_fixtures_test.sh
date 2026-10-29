#!/usr/bin/env bash
# Defect and corrected fixtures for the pinned native checks (specs/002-hermetic-skeleton T006).
# Fixture content is generated here so the repository's own scans never see a defect.
set -euo pipefail
gitleaks="$(realpath "$1")"
typos="$(realpath "$2")"
suppression="$(realpath "$3")"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0

expect() { # expect <0|1> <label> <command...>
  local want="$1" label="$2"
  shift 2
  local got=0
  "$@" </dev/null >"$work/out" 2>&1 || got=1
  if [[ "$got" != "$want" ]]; then
    echo "FAIL $label: exit class $got, want $want" >&2
    sed 's/^/  /' "$work/out" >&2
    failures=$((failures + 1))
  else
    echo "ok   $label"
  fi
}

mkdir -p "$work/secret-bad" "$work/secret-good" "$work/typos-bad" "$work/typos-good" "$work/supp"
prefix="ghp_"
printf 'token = "%s%s"\n' "$prefix" "aB3dE6gH9jK2mN5pQ8sT1vW4yZ7cF0hJ3lN6" >"$work/secret-bad/config.txt"
printf 'token = read_from_environment()\n' >"$work/secret-good/config.txt"
expect 1 "secrets defect" "$gitleaks" dir --no-banner --redact --exit-code 1 "$work/secret-bad"
expect 0 "secrets corrected" "$gitleaks" dir --no-banner --redact --exit-code 1 "$work/secret-good"

printf 'The %s%s is configured.\n' "envir" "onemnt" >"$work/typos-bad/notes.txt"
printf 'The environment is configured.\n' >"$work/typos-good/notes.txt"
expect 1 "typos defect" "$typos" "$work/typos-bad"
expect 0 "typos corrected" "$typos" "$work/typos-good"

printf 'package x\n\nfunc f() {} //%s:unused\n' "nolint" >"$work/supp/bad.go"
printf 'package x\n\nfunc f() {}\n' >"$work/supp/good.go"
printf 'bad.go\0' >"$work/bad.list"
printf 'good.go\0' >"$work/good.list"
expect 1 "suppression-ban defect" "$suppression" -root "$work/supp" -paths "$work/bad.list"
expect 0 "suppression-ban corrected" "$suppression" -root "$work/supp" -paths "$work/good.list"

exit "$failures"
