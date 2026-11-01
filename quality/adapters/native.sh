#!/usr/bin/env bash
# The one definition of how each native check is invoked (specs/002-hermetic-skeleton T026, T027,
# T034). Every tool runs with its implicit configuration disabled, so a stray config file, a host
# rc file or a helper on PATH cannot change the verdict. The verifier and the fixtures both use
# this script. Tool paths come from the environment, as the runner passes them.
#
#   quality/adapters/native.sh <check> <arguments>
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
  format-proto | lint-proto | proto-breaking) # <schema dir> [<marker>]
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
      # From inside the assembled schema, so a diagnostic names the file, not this scratch directory.
      format-proto) cd "$scratch/new" && buf format . --exclude-path buf --diff --exit-code ;;
      lint-proto) cd "$scratch/new" && buf lint . --exclude-path buf --error-format=json ;;
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
    # --verbose names the file; --redact keeps the finding out of the output. Scanning from inside
    # the snapshot keeps the reported paths relative to the repository.
    cd "$snapshot"
    "$GITLEAKS" dir --no-banner --redact --verbose --no-color --exit-code 1 --ignore-gitleaks-allow \
      --gitleaks-ignore-path /dev/null --config "$config" .
    ;;
  typos) # <newline list> <config>
    [[ -s "$1" ]] || { echo "typos: empty input list" >&2; exit 1; }
    "$TYPOS" --isolated --config "$2" --file-list "$1"
    ;;
  shellcheck) # <NUL list>
    on_files "$1" env -u SHELLCHECK_OPTS "$SHELLCHECK" --norc
    ;;
  format-starlark) # <NUL list>
    on_files "$1" "$BUILDIFIER" --config=off --mode=check --lint=warn
    ;;
  format-go) # <NUL list>
    found="$(on_files "$1" "$GOFMT" -l)"
    if [[ -n "$found" ]]; then
      echo "format-go: run gofmt -w on:" >&2
      echo "$found" >&2
      exit 1
    fi
    ;;
  format-ts) # <NUL list>
    on_files "$1" env BAZEL_BINDIR=. JS_BINARY__NO_CD_BINDIR=1 JS_BINARY__SILENT_ON_SUCCESS=1 RUNFILES="$RUNFILES_ROOT" "$PRETTIER" \
      --check --no-config --no-editorconfig --print-width 100 --log-level warn
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
