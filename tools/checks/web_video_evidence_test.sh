#!/usr/bin/env bash
# Failed-publication and provenance-bound baseline CLI controls for tools/checks/web_video_evidence.sh (#27).
# Proves unvalidated or partial media never enters the published artifact directory while failure
# logs and command-result.json are preserved, and comparative mode builds and cryptographically binds
# the actual baseline revision in a detached git worktree (or refuses missing/unprovenanced baselines).
set -euo pipefail
script="$(realpath "$1")"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

repo="$work/repo"
mkdir -p "$repo/tools/checks" "$repo/tools/bootstrap"
cp "$script" "$repo/tools/checks/web_video_evidence.sh"
chmod +x "$repo/tools/checks/web_video_evidence.sh"

cat >"$repo/tools/bootstrap/bazel.sh" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "build" ]]; then
  if [[ "${STUB_BUILD_EXIT:-0}" != 0 ]]; then
    echo "stub baseline build failed (exit=${STUB_BUILD_EXIT})" >&2
    exit "${STUB_BUILD_EXIT}"
  fi
  mkdir -p bazel-bin/web/app/dist_dev
  case "${STUB_BUILD_MODE:-normal}" in
    normal)
      rev="$(git rev-parse HEAD)"
      printf '<html><body>baseline-%s</body></html>\n' "$rev" >bazel-bin/web/app/dist_dev/index.html
      ;;
    broken-symlink)
      ln -s /nonexistent/broken-baseline-asset bazel-bin/web/app/dist_dev/index.html
      ;;
    empty-dist)
      ;;
    provenance-open-fail)
      rev="$(git rev-parse HEAD)"
      printf '<html><body>baseline-%s</body></html>\n' "$rev" >bazel-bin/web/app/dist_dev/index.html
      mkdir -p ../before-provenance.json
      ;;
  esac
  exit 0
fi
out=""
before_dist=""
before_prov=""
printf '%s\n' "$@" >"$STUB_ARGS_LOG"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --out)
      out="$2"
      shift 2
      ;;
    --before-web-dist)
      before_dist="$2"
      shift 2
      ;;
    --before-provenance)
      before_prov="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [[ -n "$before_prov" && -n "${STUB_CAPTURE_PROV:-}" ]]; then
  cp -- "$before_prov" "$STUB_CAPTURE_PROV"
fi
if [[ -n "$before_dist" && -n "${STUB_CAPTURE_DIST_INDEX:-}" ]]; then
  cp -- "$before_dist/index.html" "$STUB_CAPTURE_DIST_INDEX"
fi
mkdir -p "$out/desktop/smoke"
printf 'partial-webm' >"$out/desktop/smoke/flow.webm"
printf 'partial-png' >"$out/desktop/smoke/01-start.png"
printf '{"stub":true}\n' >"$out/manifest.json"
echo "stub recorder output (exit=${STUB_EXIT})"
exit "${STUB_EXIT}"
STUB
chmod +x "$repo/tools/bootstrap/bazel.sh"

git -C "$repo" init -q
git -C "$repo" config user.name "Test"
git -C "$repo" config user.email "test@example.invalid"
git -C "$repo" add tools
git -C "$repo" commit -q -m "baseline"
base_rev="$(git -C "$repo" rev-parse HEAD)"
base_tree="$(git -C "$repo" rev-parse "${base_rev}^{tree}")"
printf 'candidate-marker\n' >"$repo/candidate.txt"
git -C "$repo" add candidate.txt
git -C "$repo" commit -q -m "candidate"
head_rev="$(git -C "$repo" rev-parse HEAD)"

# 1. Failed recording/validation must publish ZERO media/manifest while preserving logs and result.
fail_out="$work/fail-out"
mkdir -p "$fail_out/desktop/smoke"
printf 'stale-media' >"$fail_out/desktop/smoke/flow.webm"
printf '{"stale":true}\n' >"$fail_out/manifest.json"
rc=0
STUB_EXIT=9 STUB_ARGS_LOG="$work/fail-args" \
  "$repo/tools/checks/web_video_evidence.sh" --out "$fail_out" >/dev/null 2>&1 || rc=$?
[[ "$rc" -eq 9 ]] || { echo "FAIL failed run exited $rc, want 9" >&2; exit 1; }
[[ -f "$fail_out/video-evidence-command.log" ]] || { echo "FAIL missing failure log" >&2; exit 1; }
grep -q 'stub recorder output (exit=9)' "$fail_out/video-evidence-command.log" || { echo "FAIL failure log missing output" >&2; exit 1; }
grep -q '"published":false' "$fail_out/command-result.json" || { echo "FAIL expected published:false" >&2; exit 1; }
[[ ! -e "$fail_out/manifest.json" && ! -e "$fail_out/desktop" && ! -e "$fail_out/mobile" ]] || {
  echo "FAIL unvalidated or stale media leaked into published artifact dir" >&2
  exit 1
}

# 2. Successful representative-smoke publishes manifest and media with published:true.
pass_out="$work/pass-out"
STUB_EXIT=0 STUB_ARGS_LOG="$work/pass-args" \
  "$repo/tools/checks/web_video_evidence.sh" --out "$pass_out" >/dev/null
[[ -f "$pass_out/manifest.json" && -f "$pass_out/desktop/smoke/flow.webm" && -f "$pass_out/desktop/smoke/01-start.png" ]] || {
  echo "FAIL verified media not published on success" >&2
  exit 1
}
grep -q "\"exit\":0,\"mode\":\"representative-smoke\",\"published\":true,\"revision\":\"$head_rev\",\"beforeRevision\":null" "$pass_out/command-result.json" || {
  echo "FAIL unexpected command-result.json: $(cat "$pass_out/command-result.json")" >&2
  exit 1
}

# 3. Malformed comparative arguments (missing/invalid SHA or unsupported --before-web-dist) are refused side-effect free.
for bad_case in \
  "--mode before-after" \
  "--mode before-after --before-revision not-a-sha" \
  "--mode before-after --before-revision $base_rev --before-web-dist $work"; do
  bad_out="$work/bad-out"
  rm -rf "$bad_out" "$work/bad-args"
  read -r -a bad_args <<<"$bad_case"
  rc=0
  STUB_EXIT=0 STUB_ARGS_LOG="$work/bad-args" \
    "$repo/tools/checks/web_video_evidence.sh" --out "$bad_out" "${bad_args[@]}" >/dev/null 2>&1 || rc=$?
  [[ "$rc" -eq 2 ]] || { echo "FAIL '$bad_case' exited $rc, want 2" >&2; exit 1; }
  [[ ! -e "$work/bad-args" && ! -e "$bad_out" ]] || { echo "FAIL malformed '$bad_case' had side effects" >&2; exit 1; }
done

# 3b. Well-formed baseline SHA absent from local git history must clean any prior published media,
# preserve the lookup diagnostic, and overwrite the prior receipt with exit 2 and published:false.
missing_rev="1111111111111111111111111111111111111111"
missing_out="$work/missing-base-out"
STUB_EXIT=0 STUB_ARGS_LOG="$work/missing-seed-args" \
  "$repo/tools/checks/web_video_evidence.sh" --out "$missing_out" >/dev/null
[[ -f "$missing_out/manifest.json" && -f "$missing_out/desktop/smoke/flow.webm" ]] || {
  echo "FAIL seed success run did not publish media before missing-baseline check" >&2
  exit 1
}
rc=0
STUB_EXIT=0 STUB_ARGS_LOG="$work/missing-base-args" \
  "$repo/tools/checks/web_video_evidence.sh" \
    --out "$missing_out" \
    --mode before-after \
    --before-revision "$missing_rev" >/dev/null 2>&1 || rc=$?
[[ "$rc" -eq 2 ]] || { echo "FAIL missing baseline exited $rc, want 2" >&2; exit 1; }
[[ ! -e "$work/missing-base-args" ]] || { echo "FAIL recorder ran for unavailable baseline" >&2; exit 1; }
grep -q "web_video_evidence: baseline commit $missing_rev not found in local git history" "$missing_out/video-evidence-command.log" || {
  echo "FAIL missing baseline lookup diagnostic absent from log: $(cat "$missing_out/video-evidence-command.log")" >&2
  exit 1
}
grep -q "\"exit\":2,\"mode\":\"before-after\",\"published\":false,\"revision\":\"$head_rev\",\"beforeRevision\":\"$missing_rev\"" "$missing_out/command-result.json" || {
  echo "FAIL missing baseline did not overwrite prior receipt cleanly: $(cat "$missing_out/command-result.json")" >&2
  exit 1
}
[[ ! -e "$missing_out/manifest.json" && ! -e "$missing_out/desktop" && ! -e "$missing_out/mobile" ]] || {
  echo "FAIL stale media survived unavailable baseline failure" >&2
  exit 1
}
[[ "$(git -C "$repo" worktree list --porcelain | grep -c '^worktree ')" -eq 1 ]] || {
  echo "FAIL detached worktree leaked after unavailable baseline failure" >&2
  exit 1
}

# 4. Comparative before-after mode with a real baseline commit builds in a detached worktree and emits exact provenance.
cmp_out="$work/cmp-out"
STUB_EXIT=0 STUB_ARGS_LOG="$work/cmp-args" \
  STUB_CAPTURE_PROV="$work/captured-prov.json" \
  STUB_CAPTURE_DIST_INDEX="$work/captured-index.html" \
  "$repo/tools/checks/web_video_evidence.sh" \
    --out "$cmp_out" \
    --mode before-after \
    --before-revision "$base_rev" >/dev/null
grep -qxF -- "--before-revision" "$work/cmp-args" || { echo "FAIL missing --before-revision arg" >&2; exit 1; }
grep -qxF -- "--before-tree" "$work/cmp-args" || { echo "FAIL missing --before-tree arg" >&2; exit 1; }
grep -qxF -- "$base_tree" "$work/cmp-args" || { echo "FAIL missing baseline tree sha arg" >&2; exit 1; }
grep -qxF -- "--before-provenance" "$work/cmp-args" || { echo "FAIL missing --before-provenance arg" >&2; exit 1; }
grep -qF "baseline-$base_rev" "$work/captured-index.html" || { echo "FAIL baseline build did not run at detached base commit" >&2; exit 1; }
grep -q "\"revision\":\"$base_rev\"" "$work/captured-prov.json" || { echo "FAIL provenance missing baseline revision" >&2; exit 1; }
grep -q "\"tree\":\"$base_tree\"" "$work/captured-prov.json" || { echo "FAIL provenance missing baseline tree" >&2; exit 1; }
grep -q "\"beforeRevision\":\"$base_rev\"" "$cmp_out/command-result.json" || { echo "FAIL command-result.json missing beforeRevision" >&2; exit 1; }
# Detached worktree must be cleaned up after exit.
[[ "$(git -C "$repo" worktree list --porcelain | grep -c '^worktree ')" -eq 1 ]] || {
  echo "FAIL detached baseline worktree leaked" >&2
  exit 1
}

# 5. Baseline build failure with a prior success receipt in output dir must preserve baseline diagnostics,
# overwrite the prior receipt with published:false, remove stale media, and clean up the worktree.
base_fail_out="$work/base-fail-out"
mkdir -p "$base_fail_out/desktop/smoke"
printf 'stale-media' >"$base_fail_out/desktop/smoke/flow.webm"
printf '{"stale":true}\n' >"$base_fail_out/manifest.json"
printf '{"exit":0,"mode":"representative-smoke","published":true,"revision":"%s","beforeRevision":null}\n' "$head_rev" >"$base_fail_out/command-result.json"
rc=0
STUB_BUILD_EXIT=17 STUB_EXIT=0 STUB_ARGS_LOG="$work/base-fail-args" \
  "$repo/tools/checks/web_video_evidence.sh" \
    --out "$base_fail_out" \
    --mode before-after \
    --before-revision "$base_rev" >/dev/null 2>&1 || rc=$?
[[ "$rc" -eq 17 ]] || { echo "FAIL baseline build failure exited $rc, want 17" >&2; exit 1; }
[[ ! -e "$work/base-fail-args" ]] || { echo "FAIL recorder should not run when baseline build fails" >&2; exit 1; }
[[ -f "$base_fail_out/video-evidence-command.log" ]] || { echo "FAIL baseline failure log missing" >&2; exit 1; }
grep -q 'stub baseline build failed (exit=17)' "$base_fail_out/video-evidence-command.log" || { echo "FAIL baseline diagnostic missing from log" >&2; exit 1; }
grep -q "\"exit\":17,\"mode\":\"before-after\",\"published\":false,\"revision\":\"$head_rev\",\"beforeRevision\":\"$base_rev\"" "$base_fail_out/command-result.json" || {
  echo "FAIL baseline failure did not overwrite prior receipt cleanly: $(cat "$base_fail_out/command-result.json")" >&2
  exit 1
}
[[ ! -e "$base_fail_out/manifest.json" && ! -e "$base_fail_out/desktop" && ! -e "$base_fail_out/mobile" ]] || {
  echo "FAIL stale media survived baseline build failure" >&2
  exit 1
}
[[ "$(git -C "$repo" worktree list --porcelain | grep -c '^worktree ')" -eq 1 ]] || {
  echo "FAIL detached baseline worktree leaked after build failure" >&2
  exit 1
}

# 6. Failed baseline preparation after a successful build (copy failure on broken symlink,
# digest failure on empty dist, and provenance-file open failure) must route through the shared
# finalizer, preserve phase-specific diagnostics in the retained log, overwrite any prior success
# receipt with published:false, remove stale media, and clean up the worktree.
for prep_mode in broken-symlink empty-dist provenance-open-fail; do
  prep_out="$work/prep-fail-$prep_mode"
  prep_args="$work/prep-args-$prep_mode"
  rm -rf "$prep_out" "$prep_args"
  mkdir -p "$prep_out/desktop/smoke"
  printf 'stale-media' >"$prep_out/desktop/smoke/flow.webm"
  printf '{"stale":true}\n' >"$prep_out/manifest.json"
  printf '{"exit":0,"mode":"representative-smoke","published":true,"revision":"%s","beforeRevision":null}\n' "$head_rev" >"$prep_out/command-result.json"
  rc=0
  STUB_BUILD_EXIT=0 STUB_BUILD_MODE="$prep_mode" STUB_EXIT=0 STUB_ARGS_LOG="$prep_args" \
    "$repo/tools/checks/web_video_evidence.sh" \
      --out "$prep_out" \
      --mode before-after \
      --before-revision "$base_rev" >/dev/null 2>&1 || rc=$?
  [[ "$rc" -ne 0 ]] || { echo "FAIL prep mode $prep_mode unexpectedly exited 0" >&2; exit 1; }
  [[ ! -e "$prep_args" ]] || { echo "FAIL recorder ran after failed prep mode $prep_mode" >&2; exit 1; }
  [[ -s "$prep_out/video-evidence-command.log" ]] || { echo "FAIL prep mode $prep_mode log empty or missing" >&2; exit 1; }
  case "$prep_mode" in
    broken-symlink)
      grep -q 'No such file or directory' "$prep_out/video-evidence-command.log" || {
        echo "FAIL prep mode $prep_mode missing copy diagnostic in log: $(cat "$prep_out/video-evidence-command.log")" >&2
        exit 1
      }
      ;;
    empty-dist)
      grep -q 'web_video_evidence: distribution directory is empty:' "$prep_out/video-evidence-command.log" || {
        echo "FAIL prep mode $prep_mode missing empty-dist diagnostic in log: $(cat "$prep_out/video-evidence-command.log")" >&2
        exit 1
      }
      ;;
    provenance-open-fail)
      grep -q 'Is a directory' "$prep_out/video-evidence-command.log" || {
        echo "FAIL prep mode $prep_mode missing shell open diagnostic in log: $(cat "$prep_out/video-evidence-command.log")" >&2
        exit 1
      }
      grep -q 'web_video_evidence: failed to write baseline provenance manifest:' "$prep_out/video-evidence-command.log" || {
        echo "FAIL prep mode $prep_mode missing provenance phase diagnostic in log: $(cat "$prep_out/video-evidence-command.log")" >&2
        exit 1
      }
      ;;
  esac
  grep -q "\"exit\":$rc,\"mode\":\"before-after\",\"published\":false,\"revision\":\"$head_rev\",\"beforeRevision\":\"$base_rev\"" "$prep_out/command-result.json" || {
    echo "FAIL prep mode $prep_mode did not write fresh published:false receipt: $(cat "$prep_out/command-result.json")" >&2
    exit 1
  }
  [[ ! -e "$prep_out/manifest.json" && ! -e "$prep_out/desktop" && ! -e "$prep_out/mobile" ]] || {
    echo "FAIL stale media survived failed prep mode $prep_mode" >&2
    exit 1
  }
  [[ "$(git -C "$repo" worktree list --porcelain | grep -c '^worktree ')" -eq 1 ]] || {
    echo "FAIL detached baseline worktree leaked after failed prep mode $prep_mode" >&2
    exit 1
  }
done

echo "ok   web_video_evidence_script_test"
