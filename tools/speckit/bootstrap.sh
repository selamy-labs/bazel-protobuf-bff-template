#!/usr/bin/env bash
# Reproducible Spec Kit scaffold for this repository.
#
#   tools/speckit/bootstrap.sh verify    # re-init from the pin (network); diff scaffold and manifest
#   tools/speckit/bootstrap.sh manifest  # rewrite tools/speckit/scaffold.sha256 from the working tree
#   tools/speckit/bootstrap.sh init      # (re)initialize in place; review the diff before committing
#
# The pin is an immutable commit of the official github/spec-kit repository (MIT).
set -euo pipefail

SPECKIT_REPO="https://github.com/github/spec-kit.git"
SPECKIT_TAG="v1.2.3"
SPECKIT_COMMIT="5ea8b1cf0a38699340a38e00d02770308997d8c0"
INIT_ARGS=(--non-interactive --integration claude --script sh --ignore-agent-tools)

root="$(git rev-parse --show-toplevel)"
specify=(uvx --from "git+${SPECKIT_REPO}@${SPECKIT_COMMIT}" specify)

# Files owned by Spec Kit. Project-authored content (constitution, specs/) is excluded.
scaffold_paths=(
  .claude/skills
  .specify/scripts
  .specify/templates
  .specify/workflows
  .specify/integrations
  .specify/integration.json
  .specify/init-options.json
  .specify/.gitignore
  .specify/memory/.constitution-template.json
)

manifest="tools/speckit/scaffold.sha256"

write_manifest() { # $1 = tree root; prints the manifest for its scaffold files
  (cd "$1" && find "${scaffold_paths[@]}" -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum)
}

case "${1:-verify}" in
  verify)
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT
    (cd "$tmp" && git init -q && "${specify[@]}" init --here --force "${INIT_ARGS[@]}" >/dev/null)
    status=0
    for p in "${scaffold_paths[@]}"; do
      if ! diff -r -I '"\(installed\|updated\)_at"' "$tmp/$p" "$root/$p" >/dev/null 2>&1; then
        echo "speckit-scaffold: drift in $p (expected ${SPECKIT_TAG} ${SPECKIT_COMMIT})" >&2
        diff -r -I '"\(installed\|updated\)_at"' "$tmp/$p" "$root/$p" | head -20 >&2 || true
        status=1
      fi
    done
    # The committed manifest must describe a pin-generated scaffold, not a hand-edited one. Files
    # carrying install timestamps are content-checked by the timestamp-tolerant diff above.
    stamped=(-e integrations/speckit.manifest.json -e integrations/claude.manifest.json -e workflows/workflow-registry.json)
    if ! diff <(write_manifest "$tmp" | grep -v "${stamped[@]}") <(grep -v "${stamped[@]}" "$root/$manifest") >/dev/null; then
      echo "speckit-scaffold: $manifest does not match the pinned scaffold (rerun: tools/speckit/bootstrap.sh manifest)" >&2
      status=1
    fi
    [[ $status -eq 0 ]] && echo "speckit-scaffold: OK (${SPECKIT_TAG} ${SPECKIT_COMMIT})"
    exit $status
    ;;
  manifest)
    write_manifest "$root" > "$root/$manifest"
    echo "wrote $manifest ($(wc -l < "$root/$manifest") files)"
    ;;
  init)
    cd "$root"
    "${specify[@]}" init --here --force "${INIT_ARGS[@]}"
    echo "Review: git diff -- .specify/memory/constitution.md must not lose project content." >&2
    ;;
  *)
    echo "usage: $0 [verify|manifest|init]" >&2
    exit 2
    ;;
esac
