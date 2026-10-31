#!/usr/bin/env bash
# Regressions for semantic Spec Kit manifest comparison, using the committed manifests. Equivalent
# reorderings must compare equal; each real change must still differ.
set -euo pipefail
export YQ
YQ="$(realpath "$1")"
normalize="$PWD/tools/speckit/normalize_json.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

speckit=.specify/integrations/speckit.manifest.json
registry=.specify/workflows/workflow-registry.json
mutate() { "$YQ" -p=json -o=json -I=2 "$2" "$1"; }
canon() { "$normalize" "$1" "$2"; }

same() { # same <label> <path> <yq expression>
  local base="$work/base" mutated="$work/mutated"
  mutate "$2" "$3" >"$mutated"
  cmp -s "$2" "$mutated" && fail "$1: mutation changed nothing, so it proves nothing"
  canon "$2" "$2" >"$base"
  [[ -s "$base" ]] || fail "$1: empty canonical form"
  canon "$2" "$mutated" | cmp -s "$base" - || fail "$1: equivalent content compared as different"
}
differs() { # differs <label> <path> <yq expression>
  local base="$work/base" mutated="$work/mutated" rc=0
  mutate "$2" "$3" >"$mutated"
  canon "$2" "$2" >"$base"
  canon "$2" "$mutated" >"$work/canon" 2>"$work/err" || rc=$?
  if [[ $rc -eq 0 ]] && cmp -s "$base" "$work/canon"; then fail "$1: real drift compared as equal"; fi
}

same "files map reordered" "$speckit" '.files |= (to_entries | reverse | from_entries)'
same "top-level keys reordered" "$speckit" 'to_entries | reverse | from_entries'
same "volatile timestamp changed" "$speckit" '.installed_at = "2000-01-01T00:00:00+00:00"'
same "registry timestamps changed" "$registry" '.workflows.speckit.installed_at = "x" | .workflows.speckit.updated_at = "y"'

differs "digest changed" "$speckit" '.files[".specify/scripts/bash/common.sh"] = "0000"'
differs "path removed" "$speckit" 'del(.files[".specify/scripts/bash/common.sh"])'
differs "path added" "$speckit" '.files["extra.sh"] = "0000"'
differs "version changed" "$speckit" '.version = "9.9.9"'
differs "new top-level key" "$speckit" '.extra = 1'
differs "timestamp key added at another path" "$speckit" '.files.installed_at = "x"'
differs "array order is meaningful" "$speckit" '.order = [1, 2]'
mutate "$speckit" '.order = [1, 2]' >"$work/a.json"
mutate "$speckit" '.order = [2, 1]' >"$work/b.json"
cmp -s <(canon "$speckit" "$work/a.json") <(canon "$speckit" "$work/b.json") && fail "array order was ignored"
differs "volatile field removed" "$speckit" 'del(.installed_at)'
differs "volatile field retyped" "$speckit" '.installed_at = 1'
differs "registry field changed" "$registry" '.workflows.speckit.version = "9.9.9"'
differs "only one registry timestamp removed" "$registry" 'del(.workflows.speckit.updated_at)'

rc=0
"$normalize" .specify/init-options.json .specify/init-options.json >/dev/null 2>"$work/err" || rc=$?
[[ $rc -eq 2 ]] || fail "an undeclared file must be refused, got exit $rc"

[[ $failures -eq 0 ]] && echo "ok   normalize-json"
exit "$failures"
