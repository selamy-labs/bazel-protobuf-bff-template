#!/usr/bin/env bash
# The one definition of how each native check is invoked (specs/002-hermetic-skeleton T026, T027,
# T034). Every tool runs with its implicit configuration disabled, so a stray config file, a host
# rc file or a helper on PATH cannot change the verdict. The verifier and the fixtures both use
# this script. Tool paths come from the environment, as produced by //:tool_paths.
#
#   tools/checks/native.sh <check> <arguments>
#
# Run from the repository root. File lists are NUL-delimited relative paths.
set -euo pipefail
check="$1"
shift

on_files() { # on_files <list> <command...>: one invocation over every listed file
  local list="$1"
  shift
  local files=()
  while IFS= read -r -d '' f; do files+=("$f"); done <"$list"
  if [[ ${#files[@]} -eq 0 ]]; then
    echo "$check: empty input list" >&2
    return 1
  fi
  "$@" "${files[@]}"
}

case "$check" in
  secrets) # <NUL list> <gitleaks config>: scans a snapshot of exactly the inventory, so build
    # output and ignored local files are out of scope by construction, with no path allowlist.
    list="$1"
    config="$(realpath "$2")"
    [[ -s "$list" ]] || { echo "secrets: empty input list" >&2; exit 1; }
    snapshot="$(mktemp -d)"
    trap 'rm -rf "$snapshot"' EXIT
    tar -cf - --null -T "$list" | tar -xf - -C "$snapshot"
    "$GITLEAKS" dir --no-banner --redact --no-color --exit-code 1 --ignore-gitleaks-allow \
      --gitleaks-ignore-path /dev/null --config "$config" "$snapshot"
    ;;
  typos) # <newline list> <config>
    [[ -s "$1" ]] || { echo "typos: empty input list" >&2; exit 1; }
    "$TYPOS" --isolated --config "$2" --file-list "$1"
    ;;
  shellcheck) # <NUL list>
    on_files "$1" env -u SHELLCHECK_OPTS "$SHELLCHECK" --norc
    ;;
  buildifier) # <NUL list>
    on_files "$1" "$BUILDIFIER" --config=off --mode=check --lint=warn
    ;;
  actionlint) # <NUL list>
    on_files "$1" "$ACTIONLINT" -no-color -config-file /dev/null -shellcheck="$SHELLCHECK" -pyflakes=
    ;;
  zizmor) # <NUL list>
    on_files "$1" "$ZIZMOR" --no-config --offline --persona=pedantic
    ;;
  *)
    echo "native: unknown check '$check'" >&2
    exit 2
    ;;
esac
