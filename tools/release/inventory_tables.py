"""Generates the enumerated appendices of docs/provenance.md from the lock files and the licenses read from
the fetched artifacts (spec 010 FR-003). Run through the repository's pinned Python. Rows it cannot resolve
stay explicit: `unresolved: <reason>`."""

import re
import sys

root, out = sys.argv[1], sys.argv[2]


def tsv(path: str) -> list[list[str]]:
    with open(path) as file:
        return [line.rstrip("\n").split("\t") for line in file if line.strip()]


npm, debs, py = tsv(sys.argv[3]), tsv(sys.argv[4]), tsv(sys.argv[5])
# Reviewed decisions for rows whose own metadata is prose or absent (spec 010 FR-004). Each names the license
# family as read from the exact notice, its evidence hash, its scope and its reviewer; a decision replaces only an
# `unresolved:` cell, and a decision for a package that is not enumerated, or without evidence, is an error.
decisions: dict[str, list[str]] = {}
for row in tsv(sys.argv[6]):
    if row[0].startswith("#"):
        continue
    # The evidence must carry a complete sha256: 64 hex digits on their own word boundaries. A well-shaped hash
    # is a necessary shape, not proof: it neither authenticates the reviewer nor proves what the artifact holds.
    if (
        len(row) != 5
        or any(not cell.strip() for cell in row)
        or not re.search(r"(^|[^0-9A-Za-z])sha256 [0-9a-f]{64}($|[^0-9A-Za-z])", row[2])
    ):
        sys.exit(
            f"license decision for {row[0]!r}: five filled fields and a complete 64-hex sha256 in the evidence"
            " are required"
        )
    if row[0] in decisions:
        sys.exit(f"license decision for {row[0]!r} is listed twice")
    decisions[row[0]] = row
enumerated = {pkg for pkg, _, _ in debs}
for pkg in decisions:
    if pkg not in enumerated:
        sys.exit(f"license decision for {pkg!r} names no enumerated Debian package")


def decided(pkg: str, lic: str) -> str:
    if pkg not in decisions:
        return lic
    if not lic.startswith("unresolved:"):
        sys.exit(f"license decision for {pkg!r} is unused: the package's own metadata already names {lic}")
    _, family, evidence, scope, reviewer = decisions[pkg]
    return f"reviewed: {family}; evidence: {evidence}; scope: {scope}; by {reviewer}"


with open(root + "/tools/browser/debs.bzl") as debs_file:
    debs_bzl = debs_file.read()
versions = {
    m.group(1): m.group(2) for m in re.finditer(r'"([^"]+)": \(\["[^"]*/[^"/]+_([^"_]+)_(?:amd64|all)\.deb"', debs_bzl)
}
lines = []
lines.append(
    "## Transitive npm packages (generated)\n\n"
    "Every package in `pnpm-lock.yaml`, with the license field of its fetched `package.json`. Scope: tools and the"
    " built page; which packages reach the page is stated in the browser and web table above.\n"
)
lines.append("| Item | Identifier | Version | License | Where pinned |\n| --- | --- | --- | --- | --- |")
for name, ver, lic in npm:
    lines.append(f"| {name} | `{name}` | {ver} | {lic} | `pnpm-lock.yaml` |")
lines.append(
    "\n## Debian packages for the browser (generated)\n\n"
    "Every package in `tools/browser/debs.bzl`, with the `License:` fields of its own copyright file where that"
    " file is machine-readable, or a reviewed decision from `tools/release/license_decisions.tsv` where it is prose"
    " or absent: the license family as read from the exact notice, the evidence hash, the scope and the reviewer."
    " A decision is metadata for the release review, no legal guarantee and no redistribution clearance. Scope:"
    " test-time only, never redistributed.\n"
)
lines.append("| Item | Identifier | Version | License | Where pinned |\n| --- | --- | --- | --- | --- |")
for pkg, _, lic in debs:
    version = versions.get(pkg, "unresolved: not in debs.bzl")
    lines.append(f"| {pkg} | `{pkg}` | {version} | {decided(pkg, lic)} | `tools/browser/debs.bzl` |")
lines.append(
    "\n## Spec Kit Python dependencies (generated)\n\n"
    "Every package in `tools/speckit/requirements.lock`, with the license from its wheel metadata in the managed"
    " cache. Scope: tools only.\n"
)
lines.append("| Item | Identifier | Version | License | Where pinned |\n| --- | --- | --- | --- | --- |")
for name, ver, lic in py:
    lines.append(f"| {name} | `{name}` | {ver} | {lic} | `tools/speckit/requirements.lock` |")
with open(out, "w") as out_file:
    out_file.write("\n".join(lines) + "\n")
