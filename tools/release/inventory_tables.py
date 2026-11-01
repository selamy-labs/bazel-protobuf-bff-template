"""Generates the enumerated appendices of docs/provenance.md from the lock files and the licenses read from
the fetched artifacts (spec 010 FR-003). Run through the repository's pinned Python. Rows it cannot resolve
stay explicit: `unresolved: <reason>`."""
import re, sys, json
root, out = sys.argv[1], sys.argv[2]
def tsv(path):
    return [l.rstrip("\n").split("\t") for l in open(path) if l.strip()]
npm, debs, py = tsv(sys.argv[3]), tsv(sys.argv[4]), tsv(sys.argv[5])
# Reviewed decisions for rows whose own metadata is prose or absent (spec 010 FR-004). Each names the license
# family as read from the exact notice, its evidence hash, its scope and its reviewer; a decision replaces only an
# `unresolved:` cell, and a decision for a package that is not enumerated, or without evidence, is an error.
decisions = {}
for row in tsv(sys.argv[6]):
    if row[0].startswith("#"):
        continue
    # The evidence must carry a complete sha256: 64 hex digits on their own word boundaries. A well-shaped hash
    # is a necessary shape, not proof: it neither authenticates the reviewer nor proves what the artifact holds.
    if len(row) != 5 or any(not cell.strip() for cell in row) or not re.search(r"(^|[^0-9A-Za-z])sha256 [0-9a-f]{64}($|[^0-9A-Za-z])", row[2]):
        sys.exit("license decision for %r: five filled fields and a complete 64-hex sha256 in the evidence are required" % row[0])
    if row[0] in decisions:
        sys.exit("license decision for %r is listed twice" % row[0])
    decisions[row[0]] = row
enumerated = {pkg for pkg, _, _ in debs}
for pkg in decisions:
    if pkg not in enumerated:
        sys.exit("license decision for %r names no enumerated Debian package" % pkg)
def decided(pkg, lic):
    if pkg not in decisions:
        return lic
    if not lic.startswith("unresolved:"):
        sys.exit("license decision for %r is unused: the package's own metadata already names %s" % (pkg, lic))
    _, family, evidence, scope, reviewer = decisions[pkg]
    return "reviewed: %s; evidence: %s; scope: %s; by %s" % (family, evidence, scope, reviewer)
debs_bzl = open(root + "/tools/browser/debs.bzl").read()
versions = {m.group(1): m.group(2) for m in re.finditer(r'"([^"]+)": \(\["[^"]*/[^"/]+_([^"_]+)_(?:amd64|all)\.deb"', debs_bzl)}
lines = []
lines.append("## Transitive npm packages (generated)\n\nEvery package in `pnpm-lock.yaml`, with the license field of its fetched `package.json`. Scope: tools and the built page; which packages reach the page is stated in the browser and web table above.\n")
lines.append("| Item | Identifier | Version | License | Where pinned |\n| --- | --- | --- | --- | --- |")
for name, ver, lic in npm:
    lines.append("| %s | `%s` | %s | %s | `pnpm-lock.yaml` |" % (name, name, ver, lic))
lines.append("\n## Debian packages for the browser (generated)\n\nEvery package in `tools/browser/debs.bzl`, with the `License:` fields of its own copyright file where that file is machine-readable, or a reviewed decision from `tools/release/license_decisions.tsv` where it is prose or absent: the license family as read from the exact notice, the evidence hash, the scope and the reviewer. A decision is metadata for the release review, no legal guarantee and no redistribution clearance. Scope: test-time only, never redistributed.\n")
lines.append("| Item | Identifier | Version | License | Where pinned |\n| --- | --- | --- | --- | --- |")
for pkg, _, lic in debs:
    lines.append("| %s | `%s` | %s | %s | `tools/browser/debs.bzl` |" % (pkg, pkg, versions.get(pkg, "unresolved: not in debs.bzl"), decided(pkg, lic)))
lines.append("\n## Spec Kit Python dependencies (generated)\n\nEvery package in `tools/speckit/requirements.lock`, with the license from its wheel metadata in the managed cache. Scope: tools only.\n")
lines.append("| Item | Identifier | Version | License | Where pinned |\n| --- | --- | --- | --- | --- |")
for name, ver, lic in py:
    lines.append("| %s | `%s` | %s | %s | `tools/speckit/requirements.lock` |" % (name, name, ver, lic))
open(out, "w").write("\n".join(lines) + "\n")
