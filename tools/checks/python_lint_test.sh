#!/usr/bin/env bash
# Witness for python_lint.sh (spec 011 FR-017): the pinned ruff over a planted tree. An unused import and an
# unformatted file are each named in the diagnostic form; the corrected control passes; a configuration found by
# walking up from the file cannot change the result; and each setup failure refuses with 11, never a pass.
#
#   python_lint_test.sh <python_lint.sh> <ruff> <ruff.toml>
set -euo pipefail
script="$PWD/$1"
ruff="$PWD/$2"
config_source="$PWD/$3"
work="$(mktemp -d)"
trap 'rm -rf "${work:?}"' EXIT
export HOME="$work/home"
mkdir -p "$HOME" "$work/tree/tools/pins"
cp "$config_source" "$work/tree/ruff.toml"
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }
list() { printf '%s\0' "$@" >"$work/list"; }
run() { rc=0; "$script" "$ruff" "$work/tree" ruff.toml "$work/list" >"$work/out" 2>"$work/err" || rc=$?; }

cat >"$work/tree/tools/pins/planted.py" <<'EOF'
import json
import sys


def main() -> None:
    print(sys.argv)


main()
EOF
printf 'x = {  "a":1 }\n' >"$work/tree/tools/pins/ugly.py"
list tools/pins/planted.py tools/pins/ugly.py
run
[[ $rc -eq 1 ]] || { fail "planted findings: exit $rc, want 1"; cat "$work/out" "$work/err" >&2; }
grep -qE '^python-lint tools/pins/planted\.py:1:8: F401 ' "$work/out" || { fail "the unused import is not named in the diagnostic form"; cat "$work/out" >&2; }
grep -qE '^python-lint tools/pins/ugly\.py:1:1: the formatter would rewrite this file' "$work/out" || fail "the unformatted file is not named"
grep -qxF 'measurement: python-lint 2 files; 2 findings' "$work/out" || fail "no measurement with the findings"

# The corrected control passes with its measurement.
sed -i '/^import json$/d' "$work/tree/tools/pins/planted.py"
printf 'x = {"a": 1}\n' >"$work/tree/tools/pins/ugly.py"
run
[[ $rc -eq 0 ]] || { fail "the control: exit $rc"; cat "$work/out" "$work/err" >&2; }
grep -qxF 'measurement: python-lint 2 files; 0 findings' "$work/out" || fail "no clean measurement"

# A configuration beside the file, or in the home directory, changes nothing: only the named one is read.
printf '[lint]\nselect = []\nignore = ["F401"]\n' >"$work/tree/tools/pins/ruff.toml"
mkdir -p "$HOME/.config/ruff" && printf '[lint]\nselect = []\n' >"$HOME/.config/ruff/ruff.toml"
printf 'import json\n' >"$work/tree/tools/pins/planted.py"
run
[[ $rc -eq 1 ]] || fail "a configuration found by walking up changed the result: exit $rc"
grep -q 'F401' "$work/out" || fail "the unused import escaped under a nearer configuration"
rm -f "$work/tree/tools/pins/ruff.toml"

# Only Python outside fixtures is judged; nothing selected is nothing judged, and it says so.
mkdir -p "$work/tree/tools/testdata"
printf 'import json\n' >"$work/tree/tools/testdata/planted.py"
list tools/pins/README.md tools/testdata/planted.py
run
[[ $rc -eq 0 ]] || fail "an empty selection: exit $rc"
grep -qF 'nothing to lint' "$work/out" || fail "an empty selection gave no reason"

# A setup failure is a refusal, never a pass.
list tools/pins/planted.py
rc=0; "$script" "$work/missing" "$work/tree" ruff.toml "$work/list" >"$work/out" 2>"$work/err" || rc=$?
[[ $rc -eq 11 ]] || fail "a missing ruff: exit $rc, want 11"
rc=0; "$script" "$ruff" "$work/tree" missing.toml "$work/list" >"$work/out" 2>"$work/err" || rc=$?
[[ $rc -eq 11 ]] || fail "a missing configuration: exit $rc, want 11"
rc=0; "$script" "$ruff" "$work/tree" ruff.toml "$work/no-list" >"$work/out" 2>"$work/err" || rc=$?
[[ $rc -eq 11 ]] || fail "a missing list: exit $rc, want 11"

if [[ $failures -ne 0 ]]; then
  exit 1
fi
echo "python-lint witness passed"
