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

# assemble_schema <schema dir> <out>: only .proto files are copied, so a buf configuration next to
# a schema is never read. The canonical policy and the one schema import are added here.
assemble_schema() {
  mkdir -p "$2/buf/validate"
  (cd "$1" && find . -name '*.proto' -not -path './testdata/*' -print0 | tar -cf - --null -T -) | tar -xf - -C "$2"
  cp "$VALIDATE_PROTO_FILE" "$2/buf/validate/validate.proto"
  cp "$BUF_CONFIG_FILE" "$2/buf.yaml"
  chmod -R u+w "$2"
}

buf() { # Runs with a scratch HOME, so user configuration and the credential file are not read.
  HOME="$buf_home" BUF_CACHE_DIR="$buf_home/cache" "$BUF" "$@"
}

case "$check" in
  proto-format | proto-lint | proto-breaking) # <schema dir> [<marker>]
    schema="$1"
    [[ -d "$schema" ]] || { echo "$check: no schema directory: $schema" >&2; exit 1; }
    scratch="$(mktemp -d)"
    trap 'rm -rf "$scratch"' EXIT
    buf_home="$scratch/home"
    mkdir -p "$buf_home"
    assemble_schema "$schema" "$scratch/new"
    [[ -n "$(find "$scratch/new" -name '*.proto' -not -path '*/buf/validate/*' -print -quit)" ]] ||
      { echo "$check: no schema files under $schema" >&2; exit 1; }
    case "$check" in
      proto-format) buf format "$scratch/new" --exclude-path "$scratch/new/buf" --diff --exit-code ;;
      proto-lint) buf lint "$scratch/new" --exclude-path "$scratch/new/buf" --error-format=json ;;
      proto-breaking)
        status="$("$BASELINE" -repo . -marker "$2" -schema "$schema" -out "$scratch/old")"
        echo "proto-breaking: $status"
        if [[ "$status" == bootstrap ]]; then
          echo "proto-breaking: compared nothing; this is the first schema"
          exit 0
        fi
        assemble_schema "$scratch/old" "$scratch/against"
        # The pinned import is the same on both sides, so only the schema can differ.
        buf breaking "$scratch/new" --against "$scratch/against" --error-format=json
        ;;
    esac
    ;;
  secrets) # <NUL list> <gitleaks config>: scans a snapshot of exactly the inventory, so build
    # output and ignored local files are out of scope by construction, and no path is allowlisted.
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
