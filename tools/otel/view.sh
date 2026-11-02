#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  --help) printf '%s\n' 'Local OTLP console viewer. Linux amd64; listens on 127.0.0.1:4318, metrics on 127.0.0.1:8888.' 'Usage: view.sh [--validate|--help]'; exit 0 ;;
  ''|--validate) ;;
  *) printf '%s\n' 'Unsupported argument; use --help.' >&2; exit 2 ;;
esac
[[ $# -le 1 ]] || exit 2
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || { printf '%s\n' 'This pin supports Linux amd64 only.' >&2; exit 2; }
umask 077
scratch=$(mktemp -d "${TMPDIR:-/tmp}/bff-otel-view.XXXXXXXX")
child=''
cleanup() {
  if [[ -n "$child" ]]; then kill -TERM "$child" 2>/dev/null || true; wait "$child" 2>/dev/null || true; fi
  rm -r -- "$scratch"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
version=0.164.0
archive="otelcol_${version}_linux_amd64.tar.gz"
sha256=5db7cf88c2e73a7fdce02b5fa967fcd5dc73bab00b871d947481918dc5093861
curl --fail --location --silent --show-error --proto '=https' --tlsv1.2 \
  "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v${version}/${archive}" -o "$scratch/$archive"
printf '%s  %s\n' "$sha256" "$scratch/$archive" | sha256sum --check --status
tar -xzf "$scratch/$archive" -C "$scratch" otelcol
# Under `bazel run` the script runs from runfiles and the source checkout is named; run directly, the configuration sits beside the script.
if [[ -n "${BUILD_WORKSPACE_DIRECTORY:-}" ]]; then
  config="$BUILD_WORKSPACE_DIRECTORY/tools/otel/collector.yaml"
else
  config="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/collector.yaml"
fi
[[ -f "$config" ]] || { printf '%s\n' "no collector configuration at $config" >&2; exit 2; }
if [[ ${1:-} == --validate ]]; then "$scratch/otelcol" validate --config="$config"; exit; fi
"$scratch/otelcol" --config="$config" &
child=$!
wait "$child"
child=''
