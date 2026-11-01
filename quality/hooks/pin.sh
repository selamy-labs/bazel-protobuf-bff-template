#!/usr/bin/env bash
# The one quality pin of a consumer (spec 008 R9). git_override's commit in MODULE.bazel is canonical,
# because Bazel can only read it there, and prek's rev is written from it.
#
#   pin.sh check [MODULE.bazel] [prek.toml]   both adapters name one remote and one full SHA
#   pin.sh sync  [MODULE.bazel] [prek.toml]   writes prek's rev from MODULE.bazel
#
# With `bazel run` it works in the checkout. As a rule it runs from the repository root.
set -euo pipefail
if [[ -n "${BUILD_WORKSPACE_DIRECTORY:-}" ]]; then cd "$BUILD_WORKSPACE_DIRECTORY"; fi
mode="${1:?usage: pin.sh check|sync [MODULE.bazel] [prek.toml]}"
module="${2:-MODULE.bazel}"
prek="${3:-prek.toml}"
fail() { echo "quality-pin: $1" >&2; exit 1; }

[[ -f "$module" ]] || fail "$module does not exist"
[[ -f "$prek" ]] || fail "$prek does not exist"
other="$(awk '/^[a-z_]+_override\(/ && !/^git_override\(/{inside = 1} inside {print} inside && /^\)/{inside = 0}' "$module")"
if grep -q 'module_name = "bff_template_quality"' <<<"$other"; then
  fail "$module replaces the module with something other than git_override"
fi
block="$(awk '/^git_override\(/{inside = 1; text = ""} inside {text = text $0 "\n"} inside && /^\)/{inside = 0; if (text ~ /module_name = "bff_template_quality"/) printf "%s", text}' "$module")"
[[ -n "$block" ]] || fail "$module has no git_override of bff_template_quality"
commit="$(sed -nE 's/^ *commit = "([^"]*)",?$/\1/p' <<<"$block")"
remote="$(sed -nE 's/^ *remote = "([^"]*)",?$/\1/p' <<<"$block")"
[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || fail "git_override's commit '$commit' is not a full SHA"
[[ -n "$remote" ]] || fail "git_override names no remote"
if grep -Eq '^repo = "local"' "$prek"; then fail "$prek has a local hook, which is not the pinned one"; fi

case "$mode" in
  check)
    count="$(grep -cF "repo = \"$remote\"" "$prek" || true)"
    [[ "$count" == 1 ]] || fail "$prek has $count repositories at $remote, and needs one"
    rev="$(awk -v want="repo = \"$remote\"" '$0 == want {getline line; print line}' "$prek" | sed -nE 's/^rev = "([^"]*)"$/\1/p')"
    [[ "$rev" == "$commit" ]] || fail "prek's rev '$rev' differs from git_override's commit $commit; run the pin target's sync"
    ;;
  sync)
    grep -qF "repo = \"$remote\"" "$prek" || fail "$prek has no repository at $remote"
    updated="$(mktemp "$prek.XXXXXX")"
    awk -v want="repo = \"$remote\"" -v commit="$commit" '
      found && /^rev = / { print "rev = \"" commit "\""; found = 0; next }
      $0 == want { found = 1 }
      { print }' "$prek" >"$updated"
    chmod --reference="$prek" "$updated"
    mv "$updated" "$prek"
    echo "quality-pin: prek's rev is $commit"
    ;;
  *)
    echo "quality-pin: unknown mode '$mode'" >&2
    exit 2
    ;;
esac
