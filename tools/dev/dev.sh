#!/usr/bin/env bash
# Runs the whole stack locally: the dev server and the page (specs/005-browser-client FR-009, spec 012 FR-020 to
# FR-022). The dev server accepts any test identity and listens only on loopback. The script needs only bash 3.2
# and POSIX tools, so a host without GNU coreutils can run the demo; the full verifier's host requirements stay
# its own.
#
#   tools/bootstrap/bazel.sh run //tools/dev            # start; Ctrl-C stops everything it started
#   tools/bootstrap/bazel.sh run //tools/dev -- --reset  # clear the local notes, then start
#
# The typed profile tools/dev/profile.json is the one place the demo's values are declared (ports, store, data
# directory, identity, seed subject); BFF_PORT, WEB_PORT, BFF_STORE and BFF_DATA_DIR override one value each, and
# the effective values are printed with their source. A missing, unknown or invalid value, or a port already in
# use, is refused by name before anything starts. The first child to exit ends the demo, and both are stopped.
set -euo pipefail
bff="$1"
web="$2"
dist="$3"
profile_bin="$4"
profile_file="$5"
shift 5
reset=false
for argument in "$@"; do
  case "$argument" in
    --reset) reset=true ;;
    *) echo "dev: unknown argument '$argument'; only --reset is accepted" >&2; exit 2 ;;
  esac
done
# The profile is validated once; its lines are `name="value" # from <source>`, safe to eval into these.
bff_port=""; web_port=""; store=""; data_dir=""; identity=""; seed_subject=""
effective="$("$profile_bin" --file "$profile_file" --env)" || exit 2
eval "$effective"
echo "dev: profile $profile_file ($identity)"
printf '%s\n' "$effective" | sed 's/^/dev:   /'
for port in "$bff_port" "$web_port"; do
  # A connect that succeeds means something already listens there; bash opens the socket itself, no tool needed.
  if (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
    echo "dev: port $port on 127.0.0.1 is already in use; stop that process or set BFF_PORT and WEB_PORT" >&2
    exit 2
  fi
done
store_args=("--store=$store")
if [ "$store" = local ]; then
  case "$data_dir" in
    /*) dir="$data_dir" ;;
    *) dir="${BUILD_WORKSPACE_DIRECTORY:-$PWD}/$data_dir" ;;
  esac
  if $reset; then
    if [ -e "$dir" ] && [ ! -d "$dir" ]; then echo "dev: $dir is not a directory; refusing to reset it" >&2; exit 2; fi
    rm -rf "$dir"
    echo "dev: reset: removed $dir"
  fi
  mkdir -p "$dir"
  store_args+=("--data-dir=$dir")
  echo "dev: notes persist in $dir until --reset"
else
  if $reset; then echo "dev: reset: the memory store holds nothing between runs"; fi
  echo "dev: notes live in memory until the demo stops"
fi
origin="http://127.0.0.1:$web_port"
children=()
stop() {
  if [ ${#children[@]} -gt 0 ]; then kill "${children[@]}" 2>/dev/null || true; fi
}
trap stop EXIT
trap 'exit 0' INT TERM

"$bff" --addr="127.0.0.1:$bff_port" --allowed-origin="$origin" "${store_args[@]}" </dev/null &
children+=("$!")
# The local demo is served at the root; the hosted example's base path is the hosted inputs' value, not this one.
"$web" --root "$PWD/$dist" --port "$web_port" --base-path / --bff-url "http://127.0.0.1:$bff_port" </dev/null &
children+=("$!")
echo "dev: open $origin (test identity: sign in as any subject, for example '$seed_subject'; the server is http://127.0.0.1:$bff_port; stop with Ctrl-C)"
# Wait for the first child to exit. `wait -n` would do it in one call, but it is bash 4.3; a poll keeps the same
# semantics on bash 3.2, and the trap above stops the other child.
while :; do
  for child in "${children[@]}"; do
    if ! kill -0 "$child" 2>/dev/null; then
      wait "$child" && status=0 || status=$?
      if [ "$status" -ne 0 ]; then echo "dev: a child exited with status $status; the demo stops" >&2; fi
      exit "$status"
    fi
  done
  sleep 0.2
done
