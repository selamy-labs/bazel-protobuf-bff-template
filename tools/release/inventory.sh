#!/usr/bin/env bash
# The provenance inventory check (spec 010 FR-002 to FR-004). Every third-party identifier the tree pins must
# appear verbatim in the inventory, every inventory row must carry a license, and a tracked file that
# carries someone else's copyright must sit under an enrolled vendored path. Exit 0 complete, 1 incomplete,
# 2 cannot read.
#
#   inventory.sh <repository root> <yq> [inventory, default docs/provenance.md]
#
# It reads with git, awk, sed and the pinned yq, and no ambient interpreter.
set -euo pipefail
root="${1:?usage: inventory.sh <root> <yq> [inventory]}"
yq="${2:?usage: inventory.sh <root> <yq> [inventory]}"
inventory="${3:-docs/provenance.md}"
[[ -x "$yq" ]] || { echo "inventory: no yq at $yq" >&2; exit 2; }
cd "$root"
[[ -f "$inventory" ]] || { echo "inventory: $inventory does not exist" >&2; exit 2; }
problems=0
problem() { echo "inventory: $1"; problems=$((problems + 1)); }
# An entry is a table cell in backticks, never prose. The backtick lives in a variable so that no pattern holds a pair.
bt='`'
cells="$(grep -E '^\| ' "$inventory" | grep -oE "${bt}[^${bt}]+${bt}" | tr -d "$bt" | tr ',' '\n' | sed 's/^ *//;s/ *$//' | sort -u)"
listed() { grep -qxF -- "$1" <<<"$cells"; }

# 1. Pinned identifiers, each from the file that pins it.
while read -r name; do listed "$name" || problem "bazel_dep $name has no inventory entry"; done < <(
  sed -nE 's/^bazel_dep\(name = "([^"]+)".*/\1/p' MODULE.bazel quality/MODULE.bazel | sort -u)
while read -r name; do listed "$name" || problem "Go module $name has no inventory entry"; done < <(
  awk '/^require \(/{inside=1; next} /^\)/{inside=0} inside && !/indirect/ && NF >= 2 {print $1}' go.mod quality/go.mod | sort -u)
while read -r name; do listed "$name" || problem "npm package $name has no inventory entry"; done < <(
  "$yq" -p json -o yaml '(.dependencies // {}) * (.devDependencies // {}) | keys | .[]' package.json | sort -u)
while read -r name; do listed "$name" || problem "pinned tool $name has no inventory entry"; done < <(
  "$yq" -p json -o yaml 'keys | .[]' quality/multitool.lock.json | grep -v '^\$' | sort -u)

# 2. Every entry table, one with an Identifier or Path column, has a License column, and every row's license cell is
#    filled. `unresolved: <reason>` is a filled, explicit finding counted below; an empty cell is incomplete.
rows_report="$(mktemp)"
awk -F'|' '
  function trim(s) { gsub(/^[ \t]+|[ \t]+$/, "", s); return s }
  /^\|/ {
    n = 0
    for (i = 2; i < NF; i++) cell[n++] = trim($i)
    if (!inTable) {
      inTable = 1; lic = -1; entry = 0
      for (i = 0; i < n; i++) { if (cell[i] == "License") lic = i; if (cell[i] == "Identifier" || cell[i] == "Path") entry = 1 }
      if (entry && lic < 0) print "table starting with " cell[0] " has no License column"
      next
    }
    sep = 1; for (i = 0; i < n; i++) if (cell[i] !~ /^[-: ]*$/) sep = 0
    if (sep || lic < 0) next
    if (cell[lic] == "") print cell[0] ": empty license cell"
    else if (tolower(cell[lic]) ~ /unknown/ && tolower(cell[lic]) !~ /^unresolved:/) print cell[0] ": license is neither a name nor an explicit unresolved finding"
    next
  }
  { inTable = 0 }
' "$inventory" >"$rows_report"
while IFS= read -r line; do problem "$line"; done <"$rows_report"
rm -f "$rows_report"

# 3. Required groups, and every lock entry enumerated: a group that is missing is incomplete.
for group in "## Transitive npm packages" "## Debian packages for the browser" "## Spec Kit Python dependencies" "## Built artifacts" "## Code generation" "## First-party prose boundary"; do
  grep -q "^$group" "$inventory" || problem "the inventory has no section '$group'"
done
while read -r name; do listed "$name" || problem "lock package $name has no inventory entry"; done < <(
  "$yq" -p yaml -o yaml '.packages | keys | .[]' pnpm-lock.yaml | sed -E 's/@[^@]+$//' | sort -u)
while read -r name; do listed "$name" || problem "Debian package $name has no inventory entry"; done < <(
  sed -nE 's/^    "([^"]+)": \(\[.*/\1/p' tools/browser/debs.bzl)
while read -r name; do listed "$name" || problem "Spec Kit dependency $name has no inventory entry"; done < <(
  sed -nE 's/^([A-Za-z0-9_.-]+)==.*/\1/p' tools/speckit/requirements.lock)

# 4. Every row names its required fields: an Identifier cell in backticks and a filled License, Scope and Where pinned.
rows_report="$(mktemp)"
awk -F'|' '
  function trim(s) { gsub(/^[ \t]+|[ \t]+$/, "", s); return s }
  /^\|/ {
    n = 0
    for (i = 2; i < NF; i++) cell[n++] = trim($i)
    if (!inTable) { inTable = 1; for (i = 0; i < n; i++) { col[cell[i]] = i; header[i] = cell[i] }; ncols = n; delete col[""]; next }
    sep = 1; for (i = 0; i < n; i++) if (cell[i] !~ /^[-: ]*$/) sep = 0
    if (sep) next
    if (n != ncols) { print cell[0] ": " n " cells, header has " ncols; next }
    for (f in required) if ((f in col) && cell[col[f]] == "") print cell[0] ": empty " f
    if (("Identifier" in col) && cell[col["Identifier"]] != "" && cell[col["Identifier"]] !~ /`/ && cell[col["Identifier"]] !~ /^see /) print cell[0] ": the identifier is not a backticked cell"
    next
  }
  { if (inTable) { delete col; delete header }; inTable = 0 }
  BEGIN { required["Identifier"]; required["Scope"]; required["Where pinned"]; required["Source"]; required["Revision"] }
' "$inventory" >"$rows_report"
while IFS= read -r line; do problem "$line"; done <"$rows_report"; rm -f "$rows_report"

# 5. A tracked file with a foreign copyright or license marker must be under an enrolled vendored path, or inside the
#    first-party prose boundary the inventory itself names. Nothing is skipped by file class.
enrolled="$(grep -E "^\| ${bt}" "$inventory" | cut -d'|' -f2 | grep -oE "${bt}[^${bt}]+${bt}" | tr -d "$bt" | sed 's/^ *//;s/\*\*$//;s/\*$//')"
prose="$(sed -n '/^## First-party prose boundary/,/^## /p' "$inventory" | grep -oE "${bt}[^${bt}]+${bt}" | tr -d "$bt" | sed 's/\*\*$//;s/\*$//')"
marked="$(mktemp)"
git grep -lIiE '^\s*(#|//|/\*|\*|<!--)?\s*(Copyright \(c\)|SPDX-License-Identifier|Licensed under the)' -- . >"$marked" || [[ $? -eq 1 ]] || { echo "inventory: cannot search the tree" >&2; exit 2; }
while IFS= read -r path; do
  covered=false
  while read -r prefix; do
    [[ -z "$prefix" ]] && continue
    case "$prefix" in
      \*/*) [[ "$path" == */"${prefix#*/}" ]] && covered=true ;;
      *) [[ "$path" == "$prefix"* ]] && covered=true ;;
    esac
  done < <(printf '%s\n%s\n' "$enrolled" "$prose")
  $covered || problem "$path carries a copyright or license marker and is under no enrolled path"
done <"$marked"; rm -f "$marked"

unresolved="$(grep -ciE '^\| .*\| *unresolved:' "$inventory" || true)"
echo "measurement: inventory $(grep -cE '^\| [^-|]' "$inventory") rows, $unresolved licenses unresolved"
if [[ $problems -eq 0 ]]; then
  if [[ "$unresolved" -eq 0 ]]; then echo "inventory: every entry present and every license named"; else echo "inventory: every entry present; $unresolved licenses stay unresolved, and the release review decides each"; fi
  exit 0
fi
echo "inventory: $problems problems"; exit 1
