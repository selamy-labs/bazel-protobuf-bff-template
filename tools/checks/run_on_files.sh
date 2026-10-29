#!/usr/bin/env bash
# run_on_files.sh <NUL-delimited list> <command...>: runs the command once over every listed file.
set -euo pipefail
list="$1"
shift
files=()
while IFS= read -r -d '' f; do files+=("$f"); done <"$list"
[[ ${#files[@]} -eq 0 ]] || exec "$@" "${files[@]}"
