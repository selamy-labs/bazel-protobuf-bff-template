#!/usr/bin/env bash
# Witness for governance.sh (spec 010 FR-009, FR-009a): the declared target passes offline, then each planted
# defect fails against a copy: a second repository, an import block, a data source, a ruleset active by default,
# a relaxed owner or name pin, and unformatted configuration. Copies dereference links, because runfiles are
# links to the source tree and an edit through one would reach it.
#
#   governance_test.sh <governance.sh> <tofu> <provider zip> <config dir>
set -euo pipefail
script="$PWD/$1"; tofu="$PWD/$2"; zip="$PWD/$3"; config="$PWD/$4"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
failures=0
replace() { local content; content="$(cat "$1")"; printf '%s\n' "${content/"$2"/"$3"}" >"$1"; }
expect() { # expect <name> <want> <mutation> [tofu]
  local name="$1" want="$2" rc=0
  rm -rf "$T/g"; cp -rL "$config" "$T/g"
  case "$3" in
    none) ;;
    second_repository) printf '\nresource "github_repository" "second" {\n  name = "other"\n}\n' >>"$T/g/main.tf" ;;
    import_block) printf '\nimport {\n  to = github_repository.template\n  id = "existing"\n}\n' >>"$T/g/main.tf" ;;
    data_source) printf '\ndata "github_repository" "existing" {\n  name = "x"\n}\n' >>"$T/g/main.tf" ;;
    active_by_default) replace "$T/g/variables.tf" 'default     = false' 'default     = true' ;;
    relax_owner) replace "$T/g/variables.tf" 'var.owner == "selamy-labs"' 'var.owner != ""' ;;
    relax_name) replace "$T/g/variables.tf" 'var.name == "bazel-protobuf-bff-template"' 'var.name != ""' ;;
    unformatted) replace "$T/g/main.tf" '  name        = var.name' '  name=var.name' ;;
    *) echo "unknown mutation $3"; exit 2 ;;
  esac
  "$script" "${4:-$tofu}" "$zip" "$T/g" >"$T/out" 2>&1 || rc=$?
  if [[ "$rc" == "$want" ]]; then echo "ok   $name"; else echo "FAIL $name: exit $rc"; tail -5 "$T/out" | sed 's/^/     /'; failures=$((failures + 1)); fi
}
expect "the declared target passes offline" 0 none
grep -q 'offline' "$T/out" || { echo "FAIL the pass is not stated"; failures=$((failures + 1)); }
expect "a second repository" 1 second_repository
expect "an import block" 1 import_block
expect "a data source" 1 data_source
expect "a ruleset active by default" 1 active_by_default
expect "the pinned owner relaxed" 1 relax_owner
expect "the pinned name relaxed" 1 relax_name
expect "unformatted configuration" 1 unformatted
expect "no tofu" 11 none "$T/absent"
expect "restored control" 0 none
exit "$failures"
