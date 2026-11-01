"""Generates the enumerated appendices of docs/provenance.md from the lock files and the licenses read from
the fetched artifacts (spec 010 FR-003). Run through the repository's pinned Python. Rows it cannot resolve
stay explicit: `unresolved: <reason>`."""
import re, sys, json
root, out = sys.argv[1], sys.argv[2]
def tsv(path):
    return [l.rstrip("\n").split("\t") for l in open(path) if l.strip()]
npm, debs, py = tsv(sys.argv[3]), tsv(sys.argv[4]), tsv(sys.argv[5])
debs_bzl = open(root + "/tools/browser/debs.bzl").read()
versions = {m.group(1): m.group(2) for m in re.finditer(r'"([^"]+)": \(\["[^"]*/[^"/]+_([^"_]+)_(?:amd64|all)\.deb"', debs_bzl)}
lines = []
lines.append("## Transitive npm packages (generated)\n\nEvery package in `pnpm-lock.yaml`, with the license field of its fetched `package.json`. Scope: tools and the built page; which packages reach the page is stated in the browser and web table above.\n")
lines.append("| Item | Identifier | Version | License | Where pinned |\n| --- | --- | --- | --- | --- |")
for name, ver, lic in npm:
    lines.append("| %s | `%s` | %s | %s | `pnpm-lock.yaml` |" % (name, name, ver, lic))
lines.append("\n## Debian packages for the browser (generated)\n\nEvery package in `tools/browser/debs.bzl`, with the `License:` fields of its own copyright file where that file is machine-readable. Scope: test-time only, never redistributed.\n")
lines.append("| Item | Identifier | Version | License | Where pinned |\n| --- | --- | --- | --- | --- |")
for pkg, _, lic in debs:
    lines.append("| %s | `%s` | %s | %s | `tools/browser/debs.bzl` |" % (pkg, pkg, versions.get(pkg, "unresolved: not in debs.bzl"), lic))
lines.append("\n## Spec Kit Python dependencies (generated)\n\nEvery package in `tools/speckit/requirements.lock`, with the license from its wheel metadata in the managed cache. Scope: tools only.\n")
lines.append("| Item | Identifier | Version | License | Where pinned |\n| --- | --- | --- | --- | --- |")
for name, ver, lic in py:
    lines.append("| %s | `%s` | %s | %s | `tools/speckit/requirements.lock` |" % (name, name, ver, lic))
open(out, "w").write("\n".join(lines) + "\n")
