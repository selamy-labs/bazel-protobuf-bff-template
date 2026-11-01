#!/usr/bin/env bash
# The one quality pin of a consumer (spec 008 R9). git_override's commit in MODULE.bazel is canonical,
# because Bazel can only read it there, and prek's rev is written from it.
#
#   pin.sh check [MODULE.bazel] [prek.toml]   both adapters name one remote and one full SHA
#   pin.sh sync  [MODULE.bazel] [prek.toml]   writes prek's rev from MODULE.bazel
#
# A shell has no TOML parser, and a search for a forbidden line misses its other legal spellings. So both
# modes accept only the form below, line by line, and refuse anything else. Blank lines and whole-line
# comments may appear anywhere. A host's own checks go in its registry, not in a second repository.
#
#   [[repos]]
#   repo = "<remote>"
#   rev = "<full SHA>"
#   hooks = [{ id = "<id>", args = ["<arg>"] }]     or that list with one hook on each line
#
# With `bazel run` it works in the checkout. As a rule it runs from the repository root.
set -euo pipefail
if [[ -n "${BUILD_WORKSPACE_DIRECTORY:-}" ]]; then cd "$BUILD_WORKSPACE_DIRECTORY"; fi
mode="${1:?usage: pin.sh check|sync [MODULE.bazel] [prek.toml]}"
module="${2:-MODULE.bazel}"
prek="${3:-prek.toml}"
fail() { echo "quality-pin: $1" >&2; exit 1; }
text='"[^"\\]*"'
hook="\\{ id = $text(, args = \\[$text(, $text)*\\])? \\}"

# Sets prek_repo and prek_rev, or refuses the file.
read_prek() {
  local number=0 state=header hooks=0 line
  while IFS= read -r line || [[ -n "$line" ]]; do
    number=$((number + 1))
    if [[ "$line" =~ ^[[:space:]]*(#.*)?$ ]]; then continue; fi
    case "$state" in
      header) [[ "$line" == '[[repos]]' ]] && state=repo || state=refused ;;
      repo)
        if [[ "$line" =~ ^repo\ =\ \"([^\"\\]*)\"$ ]]; then prek_repo="${BASH_REMATCH[1]}"; state=rev; else state=refused; fi
        ;;
      rev)
        if [[ "$line" =~ ^rev\ =\ \"([^\"\\]*)\"$ ]]; then prek_rev="${BASH_REMATCH[1]}"; state=hooks; else state=refused; fi
        ;;
      hooks)
        if [[ "$line" == 'hooks = [' ]]; then state=list
        elif [[ "$line" =~ ^hooks\ =\ \[$hook\]$ ]]; then state=end
        else state=refused; fi
        ;;
      list)
        if [[ "$line" =~ ^\ +$hook,$ ]]; then hooks=$((hooks + 1))
        elif [[ "$line" == ']' && "$hooks" -gt 0 ]]; then state=end
        else state=refused; fi
        ;;
      end) fail "$prek line $number follows the one pinned repository, and a second one or any other setting is refused" ;;
    esac
    if [[ "$state" == refused ]]; then
      fail "$prek line $number is not in the form the pin check reads (see hooks/pin.sh), so it cannot say the pin holds"
    fi
  done <"$prek"
  [[ "$state" == end ]] || fail "$prek stops before its one repository is complete"
}

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
while IFS= read -r line; do
  [[ "$line" == 'git_override(' || "$line" == ')' || "$line" =~ ^\ +(module_name|commit|remote|strip_prefix)\ =\ \"[^\"\\]*\",?$ ]] ||
    fail "$module's git_override sets more than the pin: '$line'"
done <<<"$block"
read_prek
[[ "$prek_repo" == "$remote" ]] || fail "$prek's repository '$prek_repo' is not git_override's remote $remote"

case "$mode" in
  check)
    [[ "$prek_rev" == "$commit" ]] || fail "prek's rev '$prek_rev' differs from git_override's commit $commit; run the pin target's sync"
    ;;
  sync)
    updated="$(mktemp "$prek.XXXXXX")"
    awk -v commit="$commit" '/^rev = / { print "rev = \"" commit "\""; next } { print }' "$prek" >"$updated"
    chmod --reference="$prek" "$updated"
    mv "$updated" "$prek"
    echo "quality-pin: prek's rev is $commit"
    ;;
  *)
    echo "quality-pin: unknown mode '$mode'" >&2
    exit 2
    ;;
esac
