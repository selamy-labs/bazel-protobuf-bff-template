#!/usr/bin/env bash
# Prints the canonical form of a Spec Kit JSON file that carries install timestamps, so scaffolds
# compare by meaning: Spec Kit writes these maps in an order that varies between hosts.
#
#   tools/speckit/normalize_json.sh <repository-relative path> <file>
#
# Map keys are sorted recursively. Only the volatile fields declared below are removed, and each
# must be present and a string, so a removed or retyped field is drift. Arrays keep their order, and
# every other key and value is preserved. An unknown path is refused rather than passed through.
# YQ is the pinned yq.
set -euo pipefail
path="$1"
file="$2"
case "$path" in
  .specify/integrations/speckit.manifest.json | .specify/integrations/claude.manifest.json)
    volatile=(.installed_at)
    ;;
  .specify/workflows/workflow-registry.json)
    volatile=(.workflows.speckit.installed_at .workflows.speckit.updated_at)
    ;;
  *)
    echo "normalize-json: $path has no declared volatile fields" >&2
    exit 2
    ;;
esac
for field in "${volatile[@]}"; do
  kind="$("$YQ" -p=json "$field | tag" "$file")"
  if [[ "$kind" != "!!str" ]]; then
    echo "normalize-json: $path: volatile field $field must be a string, found $kind" >&2
    exit 1
  fi
done
expression="$(printf 'del(%s) | ' "${volatile[@]}")sort_keys(..)"
"$YQ" -p=json -o=json -I=2 "$expression" "$file"
