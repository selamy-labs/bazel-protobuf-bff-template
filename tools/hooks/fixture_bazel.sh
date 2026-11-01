#!/usr/bin/env bash
# The stand-in for tools/bootstrap/bazel.sh in the hook fixtures. Only Bazel is replaced: `run` hands
# back the real runner with the real registry, adapter and tools, and `test` answers from what the
# checkout it runs in holds, so a verdict still depends on the commit being judged.
#
#   BOOTSTRAP_BROKEN  `run` fails, as a package the runner depends on would
#   BAD_REGISTRY      `run` hands the runner this file as its registry
#   BROKEN            `test` reports a failing test
#
# bazel.config, beside this file, lists: runner, catalog, registry, adapter, runfiles directory, tool flags.
set -euo pipefail
config="$(dirname "${BASH_SOURCE[0]}")/bazel.config"
mapfile -t values <"$config"
runner="${values[0]}"
catalog="${values[1]}"
registry="${values[2]}"
adapter="${values[3]}"
runfiles="${values[4]}"
tools=("${values[@]:5}")

while [[ "${1:-}" == --output_base=* || "${1:-}" == --max_idle_secs=* ]]; do shift; done
here="$(git rev-parse --show-toplevel)"
case "${1:-}" in
  --path) echo "$runner" ;;
  run)
    if [[ -e "$here/BOOTSTRAP_BROKEN" ]]; then
      echo "ERROR: the runner's package does not load"
      exit 1
    fi
    if [[ -e "$here/BAD_REGISTRY" ]]; then registry="$here/BAD_REGISTRY"; fi
    script=""
    for argument in "$@"; do
      if [[ "$argument" == --script_path=* ]]; then script="${argument#--script_path=}"; fi
    done
    {
      printf '#!/usr/bin/env bash\n'
      printf 'export BUILD_WORKSPACE_DIRECTORY=%q\n' "$here"
      printf 'cd %q\n' "$runfiles"
      printf 'exec'
      printf ' %q' "$runner" "--catalog=$catalog" "--registry=$registry" --registry-path=quality/registry.textproto \
        --contract=quality/coverage_floors.json --bazel=tools/bootstrap/bazel.sh "--adapter=$adapter" "${tools[@]}"
      printf ' "$%s"\n' '@'
    } >"${script:?no --script_path}"
    chmod +x "$script"
    ;;
  test)
    echo '//bff:dependency_rules_test PASSED in 0.1s'
    if [[ -e "$here/BROKEN" ]]; then
      echo '//src:unit_test FAILED in 0.1s'
      exit 3
    fi
    echo '//src:unit_test PASSED in 0.1s'
    ;;
  *)
    echo "fixture_bazel: unexpected: $*" >&2
    exit 2
    ;;
esac
