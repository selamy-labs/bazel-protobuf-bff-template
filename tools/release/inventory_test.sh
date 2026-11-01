#!/usr/bin/env bash
# Witness for inventory.sh (spec 010 FR-002 to FR-004): planted defects in a scratch tree, positive first.
#
#   inventory_test.sh <inventory.sh> <yq>
set -euo pipefail
S="$PWD/$1"
YQ="$PWD/$2"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fresh() {
  rm -rf "$T/r"; mkdir -p "$T/r/quality" "$T/r/vendor/thing" "$T/r/docs"; cd "$T/r"; git init -q
  printf 'module(name = "r")\nbazel_dep(name = "rules_go", version = "1.0.0")\n' >MODULE.bazel
  printf 'module(name = "q")\nbazel_dep(name = "rules_shell", version = "1.0.0")\n' >quality/MODULE.bazel
  printf 'module example.com/r\n\ngo 1.27\n\nrequire (\n\texample.org/dep v1.2.3\n\texample.org/ind v0.1.0 // indirect\n)\n' >go.mod
  printf 'module example.com/q\n\ngo 1.27\n' >quality/go.mod
  printf '{"dependencies": {"left-pad": "1.0.0"}, "devDependencies": {"vite": "1.0.0"}}\n' >package.json
  printf '{"%sschema": "x", "yq": {}}\n' '$' >quality/multitool.lock.json
  printf '# Copyright (c) 2001 Someone Else\nx\n' >vendor/thing/file.txt
  cat >docs/provenance.md <<'P'
# Provenance

## First-party prose boundary

`docs/**`, `*.md`

## Vendored

| Path | Source | Revision | License | Scope | Transformation |
| --- | --- | --- | --- | --- | --- |
| `vendor/thing/**` | upstream | 1 | MIT | source export | none |

## Built artifacts

| Artifact | Contains | Third-party content | Scope |
| --- | --- | --- | --- |
| none | — | — | — |

## Code generation

| Family | Inputs | Generators | Outputs, untracked |
| --- | --- | --- | --- |
| none | — | — | — |

## Pins

| Item | Identifier | Version | License | Scope | Where pinned |
| --- | --- | --- | --- | --- | --- |
| rules_go | `rules_go` | 1.0.0 | Apache-2.0 | tools | MODULE.bazel |
| rules_shell | `rules_shell` | 1.0.0 | Apache-2.0 | tools | quality/MODULE.bazel |
| dep | `example.org/dep` | 1.2.3 | MIT | built artifact | go.mod |
| left-pad | `left-pad` | 1.0.0 | MIT | tools | package.json |
| Vite | `vite` | 1.0.0 | MIT | tools | package.json |
| yq | `yq` | 1 | MIT | tools | multitool.lock.json |

## Transitive npm packages (generated)

| Item | Identifier | Version | License | Scope | Where pinned |
| --- | --- | --- | --- | --- | --- |
| left-pad | `left-pad` | 1.0.0 | MIT | tools | pnpm-lock.yaml |
| vite | `vite` | 1.0.0 | MIT | tools | pnpm-lock.yaml |

## Debian packages for the browser (generated)

| Item | Identifier | Version | License | Scope | Where pinned |
| --- | --- | --- | --- | --- | --- |
| none | `none` | 0 | none | tools | none |

## Spec Kit Python dependencies (generated)

| Item | Identifier | Version | License | Scope | Where pinned |
| --- | --- | --- | --- | --- | --- |
| none | `none` | 0 | none | tools | none |
P
  printf "lockfileVersion: '9.0'\n\npackages:\n\n  left-pad@1.0.0:\n    resolution: {integrity: x}\n\n  vite@1.0.0:\n    resolution: {integrity: x}\n\nsnapshots:\n" >pnpm-lock.yaml
  mkdir -p tools/browser tools/speckit; printf 'DEBS = {\n}\n' >tools/browser/debs.bzl; : >tools/speckit/requirements.lock
  git add -A; git -c user.name=a -c user.email=a@example.com commit -q -m one
}
failures=0
full_hash="0000000000000000000000000000000000000000000000000000000000000001"
# replace <file> <old> <new>: one literal replacement, so no pattern language touches the fixtures.
bt='`'
replace() { local content; content="$(cat "$1")"; printf '%s\n' "${content/"$2"/"$3"}" >"$1"; }
expect() { local name="$1" want="$2" rc=0; "$S" "$T/r" "$YQ" >"$T/out" 2>&1 || rc=$?; if [[ "$rc" == "$want" ]]; then echo "ok   $name"; else echo "FAIL $name: exit $rc"; sed 's/^/     /' "$T/out"; failures=$((failures + 1)); fi; }
fresh; expect "complete inventory passes" 0
fresh; rc=0; (cd / && "$S" "$T/r" "$YQ" >"$T/out" 2>&1) || rc=$?; if [[ "$rc" == 0 ]]; then echo "ok   the same tree passes from another working directory"; else echo "FAIL from another working directory: exit $rc"; sed 's/^/     /' "$T/out"; failures=$((failures + 1)); fi
fresh; printf 'bazel_dep(name = "rules_oci", version = "2.0.0")\n' >>MODULE.bazel; git add -A; expect "an unlisted bazel_dep" 1; grep -q 'rules_oci has no inventory entry' "$T/out" || { echo "FAIL not named"; failures=$((failures + 1)); }
fresh; replace go.mod $'\texample.org/dep v1.2.3' $'\texample.org/dep v1.2.3\n\texample.org/new v2.0.0'; expect "an unlisted Go module" 1
fresh; replace package.json '"left-pad": "1.0.0"' '"left-pad": "1.0.0", "newpkg": "1"'; expect "an unlisted npm package" 1
fresh; replace quality/multitool.lock.json '"yq": {}' '"yq": {}, "jq": {}'; expect "an unlisted pinned tool" 1
fresh; replace docs/provenance.md "| dep | ${bt}example.org/dep${bt} | 1.2.3 | MIT |" "| dep | ${bt}example.org/dep${bt} | 1.2.3 | unresolved: no license file in the release |"; expect "an explicit unresolved license is a counted finding, not a pass claim" 0; grep -q 'measurement: inventory .* 1 licenses unresolved' "$T/out" || { echo "FAIL unresolved not counted"; failures=$((failures + 1)); }; grep -q 'every license named' "$T/out" && { echo "FAIL claimed every license named"; failures=$((failures + 1)); }
fresh; replace docs/provenance.md "| dep | ${bt}example.org/dep${bt} | 1.2.3 | MIT |" "| dep | ${bt}example.org/dep${bt} | 1.2.3 | unknown |"; expect "a bare unknown is neither a name nor a stated finding" 1
fresh; replace docs/provenance.md "| dep | ${bt}example.org/dep${bt} | 1.2.3 | MIT |" "| dep | ${bt}example.org/dep${bt} | 1.2.3 |  |"; expect "an empty license cell" 1; grep -qi 'empty license cell' "$T/out" || { echo "FAIL empty cell not named"; failures=$((failures + 1)); }
fresh; replace docs/provenance.md "| rules_go | ${bt}rules_go${bt} | 1.0.0 | Apache-2.0 | tools | MODULE.bazel |" '| rules_go | rules_go | 1.0.0 | Apache-2.0 | tools | MODULE.bazel |'; expect "an identifier in prose rather than a cell" 1
fresh; printf '\nThe module rules_oci is mentioned in prose only.\n' >>docs/provenance.md; printf 'bazel_dep(name = "rules_oci", version = "2.0.0")\n' >>MODULE.bazel; git add -A; expect "prose does not count as an entry" 1
fresh; replace docs/provenance.md '| Item | Identifier | Version | License | Scope | Where pinned |' '| Item | Identifier | Version | Scope | Where pinned |'; expect "a table without a License column" 1
fresh; replace docs/provenance.md '## Debian packages for the browser (generated)' '## Debian packages, gone'; expect "a missing required group" 1
fresh; replace pnpm-lock.yaml '  vite@1.0.0:' $'  newdep@2.0.0:\n    resolution: {integrity: x}\n\n  vite@1.0.0:'; git add -A; expect "a lock package with no row" 1
fresh; rm pnpm-lock.yaml; git add -A; expect "a missing lock file is an error, not an empty enumeration" 2; grep -q 'cannot read pnpm-lock.yaml' "$T/out" || { echo "FAIL the missing file is not named"; failures=$((failures + 1)); }
fresh; printf 'packages: [\n' >pnpm-lock.yaml; git add -A; expect "a malformed lock file is an error" 2
fresh; printf '{}\n' >package.json; git add -A; expect "a package.json with no package is an error" 2
fresh; mkdir -p other; printf '# Copyright (c) 1999 Another\n' >other/x.c; git add -A; expect "a foreign copyright outside an enrolled path" 1; grep -q 'other/x.c carries' "$T/out" || { echo "FAIL path not named"; failures=$((failures + 1)); }
fresh; printf '# SPDX-License-Identifier: MIT\n' >docs/note.md; git add -A; expect "a license marker in documentation is not a vendored file" 0
fresh; replace docs/provenance.md "| dep | ${bt}example.org/dep${bt} | 1.2.3 | MIT |" "| dep | ${bt}example.org/dep${bt} | 1.2.3 | reviewed: MIT-style notice; evidence: copyright file, sha256 0000000000000000000000000000000000000000000000000000000000000001, 10 bytes; scope: test-time only; by a reviewer, 2026-11-01 |"; expect "a reviewed decision with a complete evidence hash, scope and reviewer is counted, not unresolved" 0; grep -q 'measurement: inventory .* 0 licenses unresolved, 1 reviewed decisions' "$T/out" || { echo "FAIL reviewed decision not counted"; failures=$((failures + 1)); }
fresh; replace docs/provenance.md "| dep | ${bt}example.org/dep${bt} | 1.2.3 | MIT |" "| dep | ${bt}example.org/dep${bt} | 1.2.3 | reviewed: MIT-style notice, trust me |"; expect "a reviewed decision without evidence, scope and reviewer" 1; grep -q 'complete 64-hex sha256' "$T/out" || { echo "FAIL the missing evidence is not named"; failures=$((failures + 1)); }
for bad in "sha256 " "sha256 f" "sha256 0123abcd" "sha256 ${full_hash}0" "sha256 ${full_hash%?}g"; do
  fresh; replace docs/provenance.md "| dep | ${bt}example.org/dep${bt} | 1.2.3 | MIT |" "| dep | ${bt}example.org/dep${bt} | 1.2.3 | reviewed: MIT-style notice; evidence: copyright file, ${bad}; scope: test-time only; by a reviewer, 2026-11-01 |"; expect "a reviewed decision whose hash is blank, short, overlong or not hex (${bad%% *} ${#bad} chars)" 1
done
fresh; expect "restored control passes" 0
echo "inventory fixture failures: $failures"; exit "$failures"
