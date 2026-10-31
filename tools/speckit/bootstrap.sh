#!/usr/bin/env bash
# Reproducible Spec Kit scaffold for this repository.
#
#   tools/speckit/bootstrap.sh verify    # re-init from the pin (network); diff scaffold and manifest
#   tools/speckit/bootstrap.sh manifest  # rewrite tools/speckit/scaffold.sha256 from the working tree
#   tools/speckit/bootstrap.sh init      # (re)initialize in place; review the diff before committing
#   tools/speckit/bootstrap.sh lock      # regenerate the hash-locked dependencies
#
# It uses the pinned uv from the quality module and a pinned managed Python, so no Python or uv
# installation is needed on the host. `verify` needs network access.
#
# The pin is an immutable commit of the official github/spec-kit repository (MIT).
set -euo pipefail

PYTHON_VERSION="3.12.15"
SPECKIT_REPO="https://github.com/github/spec-kit.git"
SPECKIT_TAG="v1.2.3"
SPECKIT_COMMIT="5ea8b1cf0a38699340a38e00d02770308997d8c0"
INIT_ARGS=(--non-interactive --integration claude --script sh --ignore-agent-tools)

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
lock="$root/tools/speckit/requirements.lock"
source_spec="specify-cli @ git+${SPECKIT_REPO}@${SPECKIT_COMMIT}"

# verify.sh exports the pinned uv as UV. Run directly, resolve the same pin through Bazel; fetching
# one pinned file executes nothing, so the weak-isolation config cannot affect the result.
if [[ -n "${UV:-}" ]]; then
  uv=("$UV")
else
  uv=("$root/tools/bootstrap/bazel.sh" run --config=weak-isolation --noshow_progress
    "--ui_event_filters=-info,-stdout,-stderr" @bff_template_quality//tools:uv --)
fi

if [[ -z "${YQ:-}" ]]; then
  # Same reasoning as uv above. normalize_json.sh runs YQ as one command, so give it a real path.
  YQ="$(mktemp)"
  trap 'rm -f "$YQ"' EXIT
  printf '#!/bin/sh\nexec %q run --config=weak-isolation --noshow_progress "--ui_event_filters=-info,-stdout,-stderr" @bff_template_quality//tools:yq -- "$@"\n' \
    "$root/tools/bootstrap/bazel.sh" >"$YQ"
  chmod +x "$YQ"
fi
export YQ

canonical_tree() { # canonical_tree <root> <empty dir>: copies the scaffold with stamped files normalized
  (cd "$1" && tar -cf - "${scaffold_paths[@]}") | tar -xf - -C "$2"
  local path
  for path in "${stamped[@]}"; do
    "$root/tools/speckit/normalize_json.sh" "$path" "$1/$path" >"$2/$path"
  done
}

specify_in() { # specify_in <empty directory>: installs the pinned CLI there; prints its path
  local dir="$1"
  "${uv[@]}" venv --no-config --quiet --python-preference only-managed --python "$PYTHON_VERSION" "$dir/venv" >&2
  "${uv[@]}" pip install --no-config --quiet --python "$dir/venv/bin/python" --require-hashes -r "$lock" >&2
  "${uv[@]}" pip install --no-config --quiet --python "$dir/venv/bin/python" --no-deps \
    --build-constraints "$root/tools/speckit/build-constraints.txt" "$source_spec" >&2
  echo "$dir/venv/bin/specify"
}

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

# Spec Kit writes these with install timestamps and host-dependent key order. They are compared by
# meaning (normalize_json.sh); everything else is compared byte for byte.
stamped=(
  .specify/integrations/speckit.manifest.json
  .specify/integrations/claude.manifest.json
  .specify/workflows/workflow-registry.json
)

write_manifest() { # $1 = tree root; prints the manifest for its scaffold files
  (cd "$1" && find "${scaffold_paths[@]}" -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum)
}

case "${1:-verify}" in
  verify)
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp" "$tmp".tool.*' EXIT
    mkdir "$tmp.tool.env" "$tmp.tool.want" "$tmp.tool.have"
    specify="$(specify_in "$tmp.tool.env")"
    (cd "$tmp" && git init -q && "$specify" init --here --force "${INIT_ARGS[@]}" >/dev/null)
    status=0
    canonical_tree "$tmp" "$tmp.tool.want"
    canonical_tree "$root" "$tmp.tool.have"
    if ! diff -r "$tmp.tool.want" "$tmp.tool.have" >"$tmp.tool.diff" 2>&1; then
      echo "speckit-scaffold: drift from ${SPECKIT_TAG} ${SPECKIT_COMMIT}:" >&2
      sed "s|$tmp.tool.want|<pinned>|g; s|$tmp.tool.have|<repository>|g" "$tmp.tool.diff" | head -20 >&2
      status=1
    fi
    # The committed manifest must describe a pin-generated scaffold, not a hand-edited one. Stamped
    # files have host-dependent raw hashes, and the comparison above covers their meaning.
    unstamped() { grep -vFf <(printf '  %s\n' "${stamped[@]}"); }
    if ! diff <(write_manifest "$tmp" | unstamped) <(unstamped <"$root/$manifest") >/dev/null; then
      echo "speckit-scaffold: $manifest does not match the pinned scaffold (rerun: tools/speckit/bootstrap.sh manifest)" >&2
      status=1
    fi
    if [[ $status -eq 0 ]]; then echo "speckit-scaffold: OK (${SPECKIT_TAG} ${SPECKIT_COMMIT})"; fi
    exit $status
    ;;
  manifest)
    write_manifest "$root" > "$root/$manifest"
    echo "wrote $manifest ($(wc -l < "$root/$manifest") files)"
    ;;
  init)
    tool="$(mktemp -d)"
    trap 'rm -rf "$tool"' EXIT
    specify="$(specify_in "$tool")"
    cd "$root"
    "$specify" init --here --force "${INIT_ARGS[@]}"
    echo "Review: git diff -- .specify/memory/constitution.md must not lose project content." >&2
    ;;
  lock)
    tool="$(mktemp -d)"
    trap 'rm -rf "$tool"' EXIT
    echo "$source_spec" >"$tool/in.txt"
    "${uv[@]}" pip compile --no-config --universal --python-version 3.12 --generate-hashes --no-header \
      --no-annotate --quiet "$tool/in.txt" -o "$tool/out.txt"
    { echo "# Hash-locked dependencies of specify-cli at the pinned commit (tools/speckit/bootstrap.sh lock)."
      grep -v '^specify-cli @' "$tool/out.txt"; } >"$lock"
    ;;
  *)
    echo "usage: $0 [verify|manifest|init|lock]" >&2
    exit 2
    ;;
esac
