#!/usr/bin/env bash
# Rejects implicit tool configuration (specs/002-hermetic-skeleton T026). Each native check names
# its configuration explicitly, so any of these files is either dead or an attempt to weaken a
# check. Reads a NUL-delimited repository path list from $1.
set -euo pipefail
status=0
while IFS= read -r -d '' path; do
  name="${path##*/}"
  case "$path" in
    .bazelrc | _typos.toml | .gitleaks.toml) continue ;; # the canonical, explicitly named configs
  esac
  case "$name" in
    .shellcheckrc | shellcheckrc | .typos.toml | typos.toml | _typos.toml | \
      .gitleaks.toml | gitleaks.toml | .gitleaksignore | .gitleaksbaseline | \
      zizmor.yml | zizmor.yaml | .zizmor.yml | .zizmor.yaml | \
      actionlint.yml | actionlint.yaml | .buildifier.json | .bazelrc | .bazeliskrc)
      echo "implicit-config $path:1: tool configuration outside the canonical set can silently disable a check; delete it (rerun: tools/verify.sh)"
      status=1
      ;;
  esac
done <"$1"
exit "$status"
