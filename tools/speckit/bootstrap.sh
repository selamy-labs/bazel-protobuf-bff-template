#!/usr/bin/env bash
# Reproducible Spec Kit scaffold for this repository.
#
#   tools/speckit/bootstrap.sh verify   # re-init into a temp dir and diff the vendored scaffold
#   tools/speckit/bootstrap.sh init     # (re)initialize in place; review the diff before committing
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
)

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
    [[ $status -eq 0 ]] && echo "speckit-scaffold: OK (${SPECKIT_TAG} ${SPECKIT_COMMIT})"
    exit $status
    ;;
  init)
    cd "$root"
    "${specify[@]}" init --here --force "${INIT_ARGS[@]}"
    echo "Review: git diff -- .specify/memory/constitution.md must not lose project content." >&2
    ;;
  *)
    echo "usage: $0 [verify|init]" >&2
    exit 2
    ;;
esac
