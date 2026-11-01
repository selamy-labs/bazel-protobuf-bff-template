#!/usr/bin/env bash
# Records the local demo (spec 012 FR-024, T022): starts the real stack through the one command with a scratch
# data directory, drives the real page in the pinned browser and captures frames, assembles one small GIF through
# the hash-locked encoder under the pinned uv Python, and writes the record beside it with the application
# revision. The raw frames and logs stay in the scratch directory; only the GIF and its record enter the tree.
# Regenerate after a material change to the page. Linux x86_64 only, because the pinned browser is.
#
#   tools/dev/record.sh            # writes docs/media/local-demo.gif and docs/media/local-demo.json
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"
[[ -z "$(git status --porcelain -- bff web/app web/client web/serve web/e2e tools/dev/dev.sh tools/dev/profile.json tools/dev/profile)" ]] || { echo "record: the page, the server or the launcher has uncommitted changes; record a committed revision" >&2; exit 2; }
revision="$(git rev-parse HEAD)"
scratch="$(mktemp -d)"
launcher=""
cleanup() {
  if [[ -n "$launcher" ]]; then kill -TERM -- "-$launcher" 2>/dev/null || true; wait "$launcher" 2>/dev/null || true; fi
  echo "record: raw frames and logs are in $scratch (not tracked)"
}
trap cleanup EXIT
bazel=(tools/bootstrap/bazel.sh)
export BFF_PORT="${BFF_PORT:-28080}" WEB_PORT="${WEB_PORT:-25173}" BFF_STORE=local BFF_DATA_DIR="$scratch/notes"
setsid "${bazel[@]}" run --config="${BAZEL_CONFIG:-weak-isolation}" --noshow_progress --ui_event_filters=-info,-stdout,-stderr //tools/dev -- --reset >"$scratch/launcher.log" 2>&1 </dev/null &
launcher=$!
origin="http://127.0.0.1:$WEB_PORT"
for _ in $(seq 1 1200); do
  if curl -sf --max-time 1 "$origin/config.json" -o /dev/null && curl -s --max-time 1 "http://127.0.0.1:$BFF_PORT/" -o /dev/null; then break; fi
  if ! kill -0 "$launcher" 2>/dev/null; then echo "record: the launcher exited: $(tail -5 "$scratch/launcher.log")" >&2; exit 1; fi
  sleep 0.25
done
curl -sf --max-time 1 "$origin/config.json" -o /dev/null || { echo "record: the stack did not come up: $(tail -5 "$scratch/launcher.log")" >&2; exit 1; }
"${bazel[@]}" run --config="${BAZEL_CONFIG:-weak-isolation}" --noshow_progress --ui_event_filters=-info,-stdout,-stderr //tools/dev/record:capture -- "$scratch/frames" "$origin" ana
# The encoder, under the pinned uv and a managed Python, with hashes required; nothing from the host.
uv=("${bazel[@]}" run --config="${BAZEL_CONFIG:-weak-isolation}" --noshow_progress "--ui_event_filters=-info,-stdout,-stderr" @bff_template_quality//tools:uv --)
"${uv[@]}" venv --no-config --quiet --python-preference only-managed --python 3.12.15 "$scratch/venv"
"${uv[@]}" pip install --no-config --quiet --python "$scratch/venv/bin/python" --require-hashes -r "$root/tools/dev/record/requirements.lock"
"$scratch/venv/bin/python" -I tools/dev/record/assemble.py "$scratch/frames" docs/media/local-demo.gif docs/media/local-demo.json "$revision"
echo "record: docs/media/local-demo.gif and docs/media/local-demo.json at $revision"
