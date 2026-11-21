#!/usr/bin/env bash
# Execute the real publisher with transport-only stubs. No image/registry acceptance is claimed.
set -euo pipefail
publisher="$(realpath "$1")"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
fixture="$work/source"
mkdir -p "$fixture/bin" "$fixture/tools/release" "$fixture/tools/checks" "$fixture/tools/bootstrap" "$fixture/docs/licenses" "$fixture/.verify"
cp "$publisher" "$fixture/tools/release/publish.sh"
printf 'fixture archive\n' >"$fixture/image.tar"
printf '{}\n' >"$fixture/.verify/receipt.json"
printf 'fixture inventory\n' >"$fixture/docs/provenance.md"
printf 'fixture license\n' >"$fixture/LICENSE"
printf 'fixture dependency notice\n' >"$fixture/docs/licenses/go-x509roots-BSD-3-Clause.txt"
cat >"$fixture/bin/git" <<'GIT'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == rev-parse ]] || exit 80
printf '%s\n' "$FIXTURE_REVISION"
GIT
cat >"$fixture/bin/docker" <<'DOCKER'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  login) cat >/dev/null; echo registry-login >>"$FIXTURE_MUTATIONS" ;;
  manifest) printf '{}\n' ;;
  *) exit 81 ;;
esac
DOCKER
cat >"$fixture/bin/gh" <<'GH'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1 $2" == 'release create' ]] || exit 82
echo release-create >>"$FIXTURE_MUTATIONS"
GH
cat >"$fixture/bin/yq" <<'YQ'
#!/usr/bin/env bash
exit 0
YQ
cat >"$fixture/tools/bootstrap/bazel.sh" <<'BAZEL'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  *image_push*) echo image-push >>"$FIXTURE_MUTATIONS" ;;
  *cquery*tools:yq*) printf '%s\n' "$FIXTURE_ROOT/bin/yq" ;;
  *cquery*image_tarball*) printf '%s\n' "$FIXTURE_ROOT/image.tar" ;;
esac
BAZEL
cat >"$fixture/tools/checks/image_release.sh" <<'IMAGE'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$#" >"$FIXTURE_ROOT/image-release-argc"
printf 'measurement: image %s, plain configuration, witnessed by fixture\n' "$FIXTURE_DIGEST"
IMAGE
cat >"$fixture/tools/checks/image_runtime.sh" <<'RUNTIME'
#!/usr/bin/env bash
set -euo pipefail
printf 'runtime-called\n' >>"$FIXTURE_RUNTIME_CALLS"
mkdir -p -- "$BFF_IMAGE_RUNTIME_OUTPUTS"
printf 'fixture runtime report\n' >"$BFF_IMAGE_RUNTIME_OUTPUTS/report.json"
case "$FIXTURE_RUNTIME_MODE" in
  failed) exit 9 ;;
  mismatch) printf 'measurement: runtime image sha256:%064d\n' 0 ;;
  absent) exit 0 ;;
  passed) printf 'measurement: runtime image %s\n' "$FIXTURE_DIGEST" ;;
  *) exit 83 ;;
esac
RUNTIME
chmod +x "$fixture/bin/"* "$fixture/tools/bootstrap/bazel.sh" "$fixture/tools/checks/"*.sh
export FIXTURE_ROOT="$fixture" FIXTURE_REVISION=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
export FIXTURE_DIGEST=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
export FIXTURE_MUTATIONS="$work/mutations" FIXTURE_RUNTIME_CALLS="$work/runtime-calls"
failures=0
for pair in 'passed 0 3' 'failed 9 0' 'mismatch 1 0' 'absent 1 0'; do
  read -r mode want mutations <<<"$pair"
  : >"$FIXTURE_MUTATIONS"
  : >"$FIXTURE_RUNTIME_CALLS"
  rc=0
  PATH="$fixture/bin:$PATH" FIXTURE_RUNTIME_MODE="$mode" GITHUB_TOKEN=synthetic-fixture-token bash "$fixture/tools/release/publish.sh" example/template v0.0.1-fixture >"$work/$mode.log" 2>&1 || rc=$?
  if [[ "$rc" != "$want" || "$(wc -l <"$FIXTURE_MUTATIONS")" != "$mutations" || "$(wc -l <"$FIXTURE_RUNTIME_CALLS")" != 1 ]]; then
    echo "FAIL publisher runtime $mode: status $rc want $want, registry writes $(wc -l <"$FIXTURE_MUTATIONS") want $mutations" >&2
    cat "$work/$mode.log" >&2
    failures=$((failures + 1))
  fi
  if [[ "$(cat "$fixture/image-release-argc")" != 3 ]]; then
    echo 'FAIL publisher did not use the real image-release wrapper/isolation/parser interface' >&2
    failures=$((failures + 1))
  fi
done
[[ "$failures" == 0 ]] && echo 'ok publisher runtime digest/status guards (one positive; three refusals; transport stubs only)'
exit "$failures"
