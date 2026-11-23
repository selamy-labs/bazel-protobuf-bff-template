#!/usr/bin/env bash
# Single-command Playwright video evidence recorder and validator for the Notes reference flow (#27).
# Replays synthetic data against the built web client and conforming stateful Connect fake inside an
# isolated staging directory, validates the emitted media fail-closed, and publishes videos,
# screenshots, and manifest.json into the output directory ONLY after validation succeeds. Failure
# logs and command-result.json are always preserved, including on baseline lookup, build, and
# preparation failures.
#
# In comparative --mode before-after, baseline bytes are ALWAYS built directly from a clean detached
# git worktree at --before-revision and bound by a SHA-256 provenance manifest (revision, git tree,
# Bazel target, and distSha256). No arbitrary caller-supplied baseline directory is accepted.
# Strictly test tooling; never in production.
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

out_dir="${BFF_VIDEO_EVIDENCE_OUTPUTS:-$root/.verify/web-video-evidence}"
mode="${BFF_VIDEO_EVIDENCE_MODE:-representative-smoke}"
viewports="${BFF_VIDEO_EVIDENCE_VIEWPORTS:-desktop,mobile}"
before_revision="${BFF_VIDEO_BEFORE_REVISION:-}"
allow_dirty=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out)
      out_dir="$2"
      shift 2
      ;;
    --mode)
      mode="$2"
      shift 2
      ;;
    --viewports)
      viewports="$2"
      shift 2
      ;;
    --before-revision)
      before_revision="$2"
      shift 2
      ;;
    --allow-dirty)
      allow_dirty=true
      shift
      ;;
    *)
      echo "web_video_evidence: unknown argument $1" >&2
      exit 2
      ;;
  esac
done

hash_dist_tree() {
  local dir="$1" files
  files="$(cd "$dir" && LC_ALL=C find -L . -type f)"
  if [[ -z "$files" ]]; then
    echo "web_video_evidence: distribution directory is empty: $dir" >&2
    return 1
  fi
  (
    cd "$dir" &&
      LC_ALL=C find -L . -type f -print0 |
      LC_ALL=C sort -z |
      xargs -0 sha256sum |
      sed 's|  \./|  |' |
      sha256sum |
      awk '{print $1}'
  )
}

case "$mode" in
  representative-smoke)
    if [[ -n "$before_revision" ]]; then
      echo "web_video_evidence: --before-revision requires --mode before-after" >&2
      exit 2
    fi
    ;;
  before-after)
    if [[ ! "$before_revision" =~ ^[0-9a-f]{40}$ ]]; then
      echo "web_video_evidence: --mode before-after requires a 40-hex --before-revision" >&2
      exit 2
    fi
    ;;
  *)
    echo "web_video_evidence: unsupported mode $mode (expected representative-smoke or before-after)" >&2
    exit 2
    ;;
esac

if [[ "$allow_dirty" != true ]]; then
  git diff --quiet HEAD -- . || {
    echo "web_video_evidence: tracked changes present; commit first or pass --allow-dirty for local preview" >&2
    exit 2
  }
fi

revision="$(git rev-parse --verify HEAD)"
mkdir -p -- "$out_dir"
out_dir="$(realpath -- "$out_dir")"
rm -rf -- "$out_dir/desktop" "$out_dir/mobile" "$out_dir/manifest.json" "$out_dir/command-result.json" "$out_dir/video-evidence-command.log"

stage_dir="$(mktemp -d)"
before_wt=""
trap 'if [[ -n "$before_wt" ]]; then git -C "$root" worktree remove --force "$before_wt" >/dev/null 2>&1 || true; fi; rm -rf -- "$stage_dir"' EXIT

: >"$stage_dir/video-evidence-command.log"
status=0
before_tree=""
before_web_dist=""
before_provenance=""

if [[ "$mode" == "before-after" ]]; then
  if ! git cat-file -e "${before_revision}^{commit}" 2>>"$stage_dir/video-evidence-command.log"; then
    echo "web_video_evidence: baseline commit $before_revision not found in local git history" >>"$stage_dir/video-evidence-command.log"
    status=2
  fi
  if [[ "$status" == 0 ]]; then
    before_tree="$(git rev-parse "${before_revision}^{tree}" 2>>"$stage_dir/video-evidence-command.log")" || status=$?
  fi
  build_args=(build)
  if [[ -n "${BFF_BAZEL_CONFIG:-}" ]]; then
    build_args+=(--config="$BFF_BAZEL_CONFIG")
  fi
  build_args+=(//web/app:dist_dev)
  if [[ "$status" == 0 ]]; then
    before_wt="$stage_dir/before-wt"
    git worktree add --detach "$before_wt" "$before_revision" >>"$stage_dir/video-evidence-command.log" 2>&1 || status=$?
  fi
  if [[ "$status" == 0 ]]; then
    (cd "$before_wt" && tools/bootstrap/bazel.sh "${build_args[@]}") >>"$stage_dir/video-evidence-command.log" 2>&1 || status=$?
  fi
  if [[ "$status" == 0 && ! -d "$before_wt/bazel-bin/web/app/dist_dev" ]]; then
    echo "web_video_evidence: baseline build produced no bazel-bin/web/app/dist_dev" >>"$stage_dir/video-evidence-command.log"
    status=1
  fi
  if [[ "$status" == 0 ]]; then
    before_web_dist="$stage_dir/before-dist"
    cp -RL "$before_wt/bazel-bin/web/app/dist_dev" "$before_web_dist" >>"$stage_dir/video-evidence-command.log" 2>&1 || status=$?
  fi
  before_dist_sha=""
  if [[ "$status" == 0 ]]; then
    before_dist_sha="$(hash_dist_tree "$before_web_dist" 2>>"$stage_dir/video-evidence-command.log")" || status=$?
  fi
  if [[ "$status" == 0 && ! "$before_dist_sha" =~ ^[0-9a-f]{64}$ ]]; then
    echo "web_video_evidence: invalid baseline distSha256: $before_dist_sha" >>"$stage_dir/video-evidence-command.log"
    status=1
  fi
  if [[ "$status" == 0 ]]; then
    before_provenance="$stage_dir/before-provenance.json"
    if ! {
      printf '{"schemaVersion":"bff-web-dist-provenance/v1","revision":"%s","tree":"%s","target":"//web/app:dist_dev","distSha256":"%s"}\n' \
        "$before_revision" "$before_tree" "$before_dist_sha" >"$before_provenance"
    } 2>>"$stage_dir/video-evidence-command.log"; then
      echo "web_video_evidence: failed to write baseline provenance manifest: $before_provenance" >>"$stage_dir/video-evidence-command.log"
      status=1
    fi
  fi
fi

if [[ "$status" == 0 ]]; then
  bazel_args=(run)
  if [[ -n "${BFF_BAZEL_CONFIG:-}" ]]; then
    bazel_args+=(--config="$BFF_BAZEL_CONFIG")
  fi
  bazel_args+=(
    //web/e2e:record_video_evidence
    --
    --out "$stage_dir/bundle"
    --revision "$revision"
    --mode "$mode"
    --viewports "$viewports"
    --command "tools/checks/web_video_evidence.sh"
  )
  if [[ "$mode" == "before-after" ]]; then
    bazel_args+=(
      --before-revision "$before_revision"
      --before-tree "$before_tree"
      --before-web-dist "$before_web_dist"
      --before-provenance "$before_provenance"
    )
  fi
  tools/bootstrap/bazel.sh "${bazel_args[@]}" >>"$stage_dir/video-evidence-command.log" 2>&1 || status=$?
fi

cp -- "$stage_dir/video-evidence-command.log" "$out_dir/video-evidence-command.log"
cat "$out_dir/video-evidence-command.log"

published=false
if [[ "$status" == 0 ]]; then
  if [[ ! -f "$stage_dir/bundle/manifest.json" ]]; then
    echo "web_video_evidence: staged bundle missing manifest.json" >&2
    status=1
  else
    for entry in "$stage_dir/bundle"/*; do
      [[ -e "$entry" ]] || continue
      mv -- "$entry" "$out_dir/"
    done
    published=true
  fi
fi

before_json="null"
if [[ -n "$before_revision" ]]; then
  before_json="\"$before_revision\""
fi
printf '{"exit":%s,"mode":"%s","published":%s,"revision":"%s","beforeRevision":%s}\n' \
  "$status" "$mode" "$published" "$revision" "$before_json" >"$out_dir/command-result.json"

exit "$status"
