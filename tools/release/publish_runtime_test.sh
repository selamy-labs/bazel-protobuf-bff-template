#!/usr/bin/env bash
# Causal fixtures for credentialless release preparation, tamper-refusing handoff verification,
# write-scoped pinned ORAS publication, and pinned ORAS checksum enforcement. No live push runs here.
set -euo pipefail
[[ $# -eq 5 ]] || { echo "usage: publish_runtime_test.sh <prepare.sh> <publish.sh> <verify_handoff.py> <oras.sh> <oras.pins>" >&2; exit 2; }
prepare_src="$(realpath -- "$1")"
publish_src="$(realpath -- "$2")"
verify_src="$(realpath -- "$3")"
oras_src="$(realpath -- "$4")"
pins_src="$(realpath -- "$5")"

export PYTHONDONTWRITEBYTECODE=1
work="$(mktemp -d)"
trap 'chmod -R u+w "$work" 2>/dev/null || true; rm -rf -- "$work"' EXIT

failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

fixture="$work/source"
mkdir -p "$fixture/bin" "$fixture/tools/release" "$fixture/tools/checks" "$fixture/tools/bootstrap" \
  "$fixture/docs/licenses" "$fixture/.verify" "$fixture/layout"
cp -- "$prepare_src" "$fixture/tools/release/prepare.sh"
cp -- "$publish_src" "$fixture/tools/release/publish.sh"
cp -- "$verify_src" "$fixture/tools/release/verify_handoff.py"
chmod +x "$fixture/tools/release/prepare.sh" "$fixture/tools/release/publish.sh"
printf '{}\n' >"$fixture/.verify/receipt.json"
printf 'fixture inventory\n' >"$fixture/docs/provenance.md"
printf 'fixture license\n' >"$fixture/LICENSE"
printf 'fixture dependency notice\n' >"$fixture/docs/licenses/go-x509roots-BSD-3-Clause.txt"

FIXTURE_DIGEST="$(python3 - "$fixture/layout" "$fixture/image.tar" <<'PY'
import hashlib
import io
import json
import sys
import tarfile
from pathlib import Path

layout_dir = Path(sys.argv[1])
tar_path = Path(sys.argv[2])

layer_buf = io.BytesIO()
with tarfile.open(fileobj=layer_buf, mode="w:") as tf:
    info = tarfile.TarInfo("bff")
    payload = b"synthetic-bff-binary\n"
    info.size = len(payload)
    info.mode = 0o555
    tf.addfile(info, io.BytesIO(payload))
layer_bytes = layer_buf.getvalue()
layer_hex = hashlib.sha256(layer_bytes).hexdigest()
layer_digest = f"sha256:{layer_hex}"

config_obj = {
    "architecture": "amd64",
    "os": "linux",
    "config": {"User": "65532:65532", "Entrypoint": ["/bff"]},
    "rootfs": {"type": "layers", "diff_ids": [layer_digest]},
}
config_bytes = json.dumps(config_obj, separators=(",", ":"), sort_keys=True).encode("utf-8")
config_hex = hashlib.sha256(config_bytes).hexdigest()
config_digest = f"sha256:{config_hex}"

manifest_obj = {
    "schemaVersion": 2,
    "mediaType": "application/vnd.oci.image.manifest.v1+json",
    "config": {
        "mediaType": "application/vnd.oci.image.config.v1+json",
        "digest": config_digest,
        "size": len(config_bytes),
    },
    "layers": [
        {
            "mediaType": "application/vnd.oci.image.layer.v1.tar",
            "digest": layer_digest,
            "size": len(layer_bytes),
        }
    ],
}
manifest_bytes = json.dumps(manifest_obj, separators=(",", ":"), sort_keys=True).encode("utf-8")
manifest_hex = hashlib.sha256(manifest_bytes).hexdigest()
manifest_digest = f"sha256:{manifest_hex}"

blobs = layout_dir / "blobs" / "sha256"
blobs.mkdir(parents=True, exist_ok=True)
(blobs / layer_hex).write_bytes(layer_bytes)
(blobs / config_hex).write_bytes(config_bytes)
(blobs / manifest_hex).write_bytes(manifest_bytes)
(layout_dir / "oci-layout").write_text('{"imageLayoutVersion":"1.0.0"}\n', encoding="utf-8")
index_obj = {
    "schemaVersion": 2,
    "manifests": [
        {
            "mediaType": "application/vnd.oci.image.manifest.v1+json",
            "digest": manifest_digest,
            "size": len(manifest_bytes),
        }
    ],
}
(layout_dir / "index.json").write_text(json.dumps(index_obj) + "\n", encoding="utf-8")

with tarfile.open(tar_path, mode="w:") as out_tar:
    def add_bytes(name: str, data: bytes) -> None:
        ti = tarfile.TarInfo(name)
        ti.size = len(data)
        ti.mode = 0o644
        out_tar.addfile(ti, io.BytesIO(data))

    add_bytes(f"{config_hex}.json", config_bytes)
    add_bytes(f"{layer_hex}.tar", layer_bytes)
    manifest_json = json.dumps(
        [{"Config": f"{config_hex}.json", "RepoTags": ["bff-template/bff:local"], "Layers": [f"{layer_hex}.tar"]}]
    ).encode("utf-8")
    add_bytes("manifest.json", manifest_json)

print(manifest_digest)
PY
)"
export FIXTURE_ROOT="$fixture"
export FIXTURE_REVISION=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
export FIXTURE_DIGEST
export FIXTURE_MUTATIONS="$work/mutations"
export FIXTURE_BAZEL_CALLS="$work/bazel-calls"
export FIXTURE_RUNTIME_CALLS="$work/runtime-calls"
export FIXTURE_FORBIDDEN_CALLS="$work/forbidden-calls"
export FIXTURE_GH_ARGS="$work/gh-args"
export FIXTURE_RESOLVE_DIGEST="$FIXTURE_DIGEST"

cat >"$fixture/bin/git" <<'GIT'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == rev-parse ]] || exit 80
printf '%s\n' "$FIXTURE_REVISION"
GIT

cat >"$fixture/bin/yq" <<'YQ'
#!/usr/bin/env bash
exit 0
YQ

cat >"$fixture/tools/bootstrap/bazel.sh" <<'BAZEL'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${FIXTURE_FORBID_BUILD:-0}" == "1" ]]; then
  printf 'forbidden bazel %s\n' "$*" >>"$FIXTURE_FORBIDDEN_CALLS"
  exit 99
fi
case "$1" in
  build) echo bazel-build >>"$FIXTURE_BAZEL_CALLS" ;;
  cquery)
    case "$*" in
      *tools:yq*) printf '%s\n' "$FIXTURE_ROOT/bin/yq" ;;
      *bff:image_tarball*) printf '%s\n' "$FIXTURE_ROOT/image.tar" ;;
      *bff:image*) printf '%s\n' "$FIXTURE_ROOT/layout" ;;
      *) exit 87 ;;
    esac
    ;;
  *) exit 88 ;;
esac
BAZEL

cat >"$fixture/tools/checks/image_release.sh" <<'IMAGE'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${FIXTURE_FORBID_BUILD:-0}" == "1" ]]; then
  printf 'forbidden image_release %s\n' "$*" >>"$FIXTURE_FORBIDDEN_CALLS"
  exit 99
fi
printf '%s\n' "$#" >"$FIXTURE_ROOT/image-release-argc"
printf 'measurement: image %s, plain configuration, witnessed by fixture\n' "$FIXTURE_DIGEST"
IMAGE

cat >"$fixture/tools/checks/image_runtime.sh" <<'RUNTIME'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${FIXTURE_FORBID_BUILD:-0}" == "1" ]]; then
  printf 'forbidden image_runtime %s\n' "$*" >>"$FIXTURE_FORBIDDEN_CALLS"
  exit 99
fi
printf 'runtime-called\n' >>"$FIXTURE_RUNTIME_CALLS"
[[ ! ${GITHUB_TOKEN+x} && ! ${GH_TOKEN+x} ]] || { echo 'runtime witness received a token' >&2; exit 84; }
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

cat >"$fixture/bin/docker" <<'DOCKER'
#!/usr/bin/env bash
set -euo pipefail
printf 'forbidden docker %s\n' "$*" >>"$FIXTURE_FORBIDDEN_CALLS"
exit 99
DOCKER

cat >"$fixture/tools/release/oras.sh" <<'ORAS'
#!/usr/bin/env bash
set -euo pipefail
[[ ! ${GITHUB_TOKEN+x} && ! ${GH_TOKEN+x} ]] || { echo 'oras received ambient GitHub token in env' >&2; exit 84; }
cmd="$1"; shift
case "$cmd" in
  login)
    [[ "$*" == *"--registry-config "* && "$*" == *"--password-stdin ghcr.io"* ]] || exit 85
    [[ "$(cat)" == synthetic-fixture-token ]] || exit 85
    echo oras-login >>"$FIXTURE_MUTATIONS"
    ;;
  cp)
    [[ "$1" == "--from-oci-layout" ]] || exit 86
    src_ref=""
    dst_ref=""
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --from-oci-layout|--no-tty) shift ;;
        --to-registry-config) shift 2 ;;
        *)
          if [[ -z "$src_ref" ]]; then src_ref="$1"; else dst_ref="$1"; fi
          shift ;;
      esac
    done
    src_dir="${src_ref%@*}"
    src_digest="${src_ref#*@}"
    [[ -d "$src_dir" && -f "$src_dir/index.json" && "$src_digest" == "$FIXTURE_DIGEST" ]] || exit 86
    [[ "$dst_ref" == "ghcr.io/example/template/bff:v0.0.1-fixture" ]] || exit 86
    echo oras-cp >>"$FIXTURE_MUTATIONS"
    ;;
  resolve)
    [[ "$*" == *"--registry-config "*" ghcr.io/example/template/bff:v0.0.1-fixture" ]] || exit 87
    echo oras-resolve >>"$FIXTURE_MUTATIONS"
    printf '%s\n' "$FIXTURE_RESOLVE_DIGEST"
    ;;
  *) exit 81 ;;
esac
ORAS

cat >"$fixture/bin/gh" <<'GH'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1 $2" == 'release create' ]] || exit 82
[[ "${GITHUB_TOKEN:-}" == synthetic-fixture-token ]] || exit 86
printf '%s\n' "$@" >"$FIXTURE_GH_ARGS"
echo release-create >>"$FIXTURE_MUTATIONS"
GH

chmod +x "$fixture/bin/"* "$fixture/tools/bootstrap/bazel.sh" "$fixture/tools/checks/"*.sh "$fixture/tools/release/oras.sh"

# Suite 1: credentialless prepare.sh refuses credentials and runtime failures, passes cleanly without tokens.
for pair in 'failed 9 1' 'mismatch 1 1' 'absent 1 1' 'passed 0 1'; do
  read -r mode want runtime_calls <<<"$pair"
  rm -rf "$fixture/.verify/release-handoff"
  : >"$FIXTURE_BAZEL_CALLS"
  : >"$FIXTURE_RUNTIME_CALLS"
  rc=0
  env -u GITHUB_TOKEN -u GH_TOKEN PATH="$fixture/bin:$PATH" FIXTURE_FORBID_BUILD=0 FIXTURE_RUNTIME_MODE="$mode" \
    bash "$fixture/tools/release/prepare.sh" v0.0.1-fixture "$fixture/.verify/release-handoff" >"$work/prepare-$mode.log" 2>&1 || rc=$?
  if [[ "$rc" != "$want" || "$(wc -l <"$FIXTURE_RUNTIME_CALLS")" != "$runtime_calls" ]]; then
    fail "prepare $mode: status $rc want $want, runtime calls $(wc -l <"$FIXTURE_RUNTIME_CALLS") want $runtime_calls"
    cat "$work/prepare-$mode.log" >&2
  fi
  if [[ "$mode" != passed && -d "$fixture/.verify/release-handoff" ]]; then
    fail "prepare $mode: left handoff directory on refusal"
  fi
done
if [[ "$(cat "$fixture/image-release-argc")" != 3 ]]; then
  fail "prepare did not use image_release.sh 3-arg interface"
fi

for tok_env in 'GITHUB_TOKEN=synthetic' 'GH_TOKEN=synthetic'; do
  : >"$FIXTURE_BAZEL_CALLS"
  : >"$FIXTURE_RUNTIME_CALLS"
  rc=0
  env -u GITHUB_TOKEN -u GH_TOKEN "$tok_env" PATH="$fixture/bin:$PATH" FIXTURE_FORBID_BUILD=0 FIXTURE_RUNTIME_MODE=passed \
    bash "$fixture/tools/release/prepare.sh" v0.0.1-fixture "$work/should-not-exist" >"$work/prepare-$tok_env.log" 2>&1 || rc=$?
  if [[ "$rc" != 2 || -s "$FIXTURE_BAZEL_CALLS" || -s "$FIXTURE_RUNTIME_CALLS" ]]; then
    fail "prepare with $tok_env exited $rc or ran build/runtime"
  fi
done

mv "$fixture/.verify/receipt.json" "$fixture/.verify/receipt.json.bak"
: >"$FIXTURE_BAZEL_CALLS"
: >"$FIXTURE_RUNTIME_CALLS"
rc=0
env -u GITHUB_TOKEN -u GH_TOKEN PATH="$fixture/bin:$PATH" FIXTURE_FORBID_BUILD=0 FIXTURE_RUNTIME_MODE=passed \
  bash "$fixture/tools/release/prepare.sh" v0.0.1-fixture "$work/should-not-exist" >"$work/prepare-no-receipt.log" 2>&1 || rc=$?
if [[ "$rc" != 2 || -s "$FIXTURE_BAZEL_CALLS" || -s "$FIXTURE_RUNTIME_CALLS" ]]; then
  fail "prepare without receipt exited $rc or ran build/runtime"
fi
mv "$fixture/.verify/receipt.json.bak" "$fixture/.verify/receipt.json"

# Produce clean reference handoff for Suite 2.
GOOD_HANDOFF="$work/good-handoff"
env -u GITHUB_TOKEN -u GH_TOKEN PATH="$fixture/bin:$PATH" FIXTURE_FORBID_BUILD=0 FIXTURE_RUNTIME_MODE=passed \
  bash "$fixture/tools/release/prepare.sh" v0.0.1-fixture "$GOOD_HANDOFF" >"$work/prepare-good.log" 2>&1 || fail "prepare good handoff failed"

# Suite 2: publish.sh positive path (zero build/docker/runtime) + tamper refusals BEFORE ORAS/gh + resolve mismatch.
: >"$FIXTURE_MUTATIONS"
: >"$FIXTURE_FORBIDDEN_CALLS"
: >"$FIXTURE_GH_ARGS"
rc=0
PATH="$fixture/bin:$PATH" FIXTURE_FORBID_BUILD=1 GITHUB_TOKEN=synthetic-fixture-token GITHUB_ACTOR=selamy-labs-bot \
  bash "$fixture/tools/release/publish.sh" example/template v0.0.1-fixture "$GOOD_HANDOFF" >"$work/publish-good.log" 2>&1 || rc=$?
want_seq=$'oras-login\noras-cp\noras-resolve\nrelease-create'
if [[ "$rc" != 0 || -s "$FIXTURE_FORBIDDEN_CALLS" || "$(cat "$FIXTURE_MUTATIONS")" != "$want_seq" ]]; then
  fail "publish good: rc=$rc forbidden=$(wc -l <"$FIXTURE_FORBIDDEN_CALLS") mutations=$(tr '\n' ',' <"$FIXTURE_MUTATIONS")"
  cat "$work/publish-good.log" >&2
fi
if grep -qE 'image-layout-|HANDOFF\.sha256' "$FIXTURE_GH_ARGS"; then
  fail "publish leaked internal handoff layout or HANDOFF.sha256 to GitHub release assets"
fi
if ! grep -qF 'SHA256SUMS' "$FIXTURE_GH_ARGS" || ! grep -qF 'bff-image-v0.0.1-fixture.tar' "$FIXTURE_GH_ARGS"; then
  fail "publish omitted SHA256SUMS or bff-image tarball from GitHub release assets"
fi

rehash_handoff_dir() {
  local dir="$1"
  local pub=()
  mapfile -t pub < <(python3 "$fixture/tools/release/verify_handoff.py" --list-public-assets --tag v0.0.1-fixture)
  (
    cd "$dir"
    LC_ALL=C sha256sum -- "${pub[@]}" >SHA256SUMS
    LC_ALL=C sha256sum -- "${pub[@]}" SHA256SUMS image-layout-v0.0.1-fixture.tar.gz >HANDOFF.sha256
  )
}

mutate_tamper_dir() {
  local mode="$1"
  local dir="$2"
  case "$mode" in
    bad-digest) printf 'sha256:%064d\n' 0 >"$dir/image-digest-v0.0.1-fixture.txt" ;;
    bad-rev) printf '%040d\n' 0 >"$dir/source-revision-v0.0.1-fixture.txt" ;;
    missing-handoff) rm -f "$dir/HANDOFF.sha256" ;;
    stray-file) touch "$dir/extra-untrusted.txt" ;;
    unset-token) : ;;
    rehashed-tar)
      python3 - "$dir/bff-image-v0.0.1-fixture.tar" <<'PY'
import hashlib, io, json, sys, tarfile
from pathlib import Path
p = Path(sys.argv[1])
cfg = json.dumps({"architecture": "amd64", "os": "linux", "config": {"User": "0:0"}, "rootfs": {"type": "layers", "diff_ids": []}}).encode()
ch = hashlib.sha256(cfg).hexdigest()
with tarfile.open(p, "w:") as tf:
    for n, d in [(f"{ch}.json", cfg), ("manifest.json", json.dumps([{"Config": f"{ch}.json", "Layers": []}]).encode())]:
        ti = tarfile.TarInfo(n); ti.size = len(d); tf.addfile(ti, io.BytesIO(d))
PY
      rehash_handoff_dir "$dir"
      ;;
    missing-tar-layer)
      python3 - "$dir/bff-image-v0.0.1-fixture.tar" <<'PY'
import io, json, sys, tarfile
from pathlib import Path
p = Path(sys.argv[1])
members = {}
with tarfile.open(p, "r:") as tf:
    for m in tf.getmembers():
        if m.isfile():
            members[m.name] = tf.extractfile(m).read()
manifests = json.loads(members["manifest.json"].decode("utf-8"))
cfg_name = manifests[0]["Config"]
manifests[0]["Layers"] = ["missing.tar"]
with tarfile.open(p, "w:") as tf:
    for n, d in [(cfg_name, members[cfg_name]), ("manifest.json", json.dumps(manifests).encode("utf-8"))]:
        ti = tarfile.TarInfo(n); ti.size = len(d); ti.mode = 0o644; tf.addfile(ti, io.BytesIO(d))
PY
      rehash_handoff_dir "$dir"
      ;;
    substituted-tar-layer)
      python3 - "$dir/bff-image-v0.0.1-fixture.tar" <<'PY'
import io, json, sys, tarfile
from pathlib import Path
p = Path(sys.argv[1])
members = {}
with tarfile.open(p, "r:") as tf:
    for m in tf.getmembers():
        if m.isfile():
            members[m.name] = tf.extractfile(m).read()
manifests = json.loads(members["manifest.json"].decode("utf-8"))
cfg_name = manifests[0]["Config"]
layer_name = manifests[0]["Layers"][0]
bad_layer_buf = io.BytesIO()
with tarfile.open(fileobj=bad_layer_buf, mode="w:") as ltf:
    info = tarfile.TarInfo("bff")
    payload = b"substituted-bff-binary\n"
    info.size = len(payload)
    info.mode = 0o555
    ltf.addfile(info, io.BytesIO(payload))
members[layer_name] = bad_layer_buf.getvalue()
with tarfile.open(p, "w:") as tf:
    for n in (cfg_name, layer_name, "manifest.json"):
        d = members[n]
        ti = tarfile.TarInfo(n); ti.size = len(d); ti.mode = 0o644; tf.addfile(ti, io.BytesIO(d))
PY
      rehash_handoff_dir "$dir"
      ;;
    symlink-tar-member | traversal-tar-member | duplicate-tar-member)
      python3 - "$dir/bff-image-v0.0.1-fixture.tar" "$mode" <<'PY'
import io, sys, tarfile
from pathlib import Path
p, mode = Path(sys.argv[1]), sys.argv[2]
with tarfile.open(p, "a:") as tf:
    if mode == "symlink-tar-member":
        ti = tarfile.TarInfo("evil-link")
        ti.type = tarfile.SYMTYPE
        ti.linkname = "/etc/passwd"
        tf.addfile(ti)
    elif mode == "traversal-tar-member":
        ti = tarfile.TarInfo("../escape.txt")
        ti.size = 1
        tf.addfile(ti, io.BytesIO(b"x"))
    else:
        ti = tarfile.TarInfo("manifest.json")
        ti.size = 2
        tf.addfile(ti, io.BytesIO(b"[]"))
PY
      rehash_handoff_dir "$dir"
      ;;
    *) echo "FAIL unknown tamper mode $mode" >&2; return 1 ;;
  esac
}

run_tamper_case() {
  local name="$1"
  local token="$2"
  local dir="$work/tamper-$name"
  rm -rf "$dir"
  cp -r "$GOOD_HANDOFF" "$dir"
  mutate_tamper_dir "$name" "$dir"
  : >"$FIXTURE_MUTATIONS"
  : >"$FIXTURE_FORBIDDEN_CALLS"
  local rc=0
  if [[ -n "$token" ]]; then
    PATH="$fixture/bin:$PATH" FIXTURE_FORBID_BUILD=1 GITHUB_TOKEN="$token" \
      bash "$fixture/tools/release/publish.sh" example/template v0.0.1-fixture "$dir" >"$work/tamper-$name.log" 2>&1 || rc=$?
  else
    env -u GITHUB_TOKEN PATH="$fixture/bin:$PATH" FIXTURE_FORBID_BUILD=1 \
      bash "$fixture/tools/release/publish.sh" example/template v0.0.1-fixture "$dir" >"$work/tamper-$name.log" 2>&1 || rc=$?
  fi
  if [[ "$rc" == 0 || -s "$FIXTURE_MUTATIONS" || -s "$FIXTURE_FORBIDDEN_CALLS" ]]; then
    fail "tamper $name: rc=$rc mutations=$(wc -l <"$FIXTURE_MUTATIONS") forbidden=$(wc -l <"$FIXTURE_FORBIDDEN_CALLS")"
    cat "$work/tamper-$name.log" >&2
  fi
}

run_restored_positive() {
  local name="$1"
  local dir="$work/tamper-$name"
  cp -- "$GOOD_HANDOFF/bff-image-v0.0.1-fixture.tar" "$dir/bff-image-v0.0.1-fixture.tar"
  rehash_handoff_dir "$dir"
  : >"$FIXTURE_MUTATIONS"
  : >"$FIXTURE_FORBIDDEN_CALLS"
  local rc=0
  PATH="$fixture/bin:$PATH" FIXTURE_FORBID_BUILD=1 GITHUB_TOKEN=synthetic-fixture-token GITHUB_ACTOR=selamy-labs-bot \
    bash "$fixture/tools/release/publish.sh" example/template v0.0.1-fixture "$dir" >"$work/restored-$name.log" 2>&1 || rc=$?
  if [[ "$rc" != 0 || -s "$FIXTURE_FORBIDDEN_CALLS" || "$(cat "$FIXTURE_MUTATIONS")" != "$want_seq" ]]; then
    fail "restored $name: rc=$rc forbidden=$(wc -l <"$FIXTURE_FORBIDDEN_CALLS") mutations=$(tr '\n' ',' <"$FIXTURE_MUTATIONS")"
    cat "$work/restored-$name.log" >&2
  fi
}

run_tamper_case bad-digest synthetic-fixture-token
run_tamper_case bad-rev synthetic-fixture-token
run_tamper_case missing-handoff synthetic-fixture-token
run_tamper_case stray-file synthetic-fixture-token
run_tamper_case unset-token ""
run_tamper_case rehashed-tar synthetic-fixture-token
run_tamper_case missing-tar-layer synthetic-fixture-token
grep -qF 'missing layer member' "$work/tamper-missing-tar-layer.log" || fail "missing-tar-layer diagnostic: $(cat "$work/tamper-missing-tar-layer.log")"
run_restored_positive missing-tar-layer
run_tamper_case substituted-tar-layer synthetic-fixture-token
grep -qF 'does not match diff_id' "$work/tamper-substituted-tar-layer.log" || fail "substituted-tar-layer diagnostic: $(cat "$work/tamper-substituted-tar-layer.log")"
run_restored_positive substituted-tar-layer
run_tamper_case symlink-tar-member synthetic-fixture-token
grep -qF 'unsupported tar member' "$work/tamper-symlink-tar-member.log" || fail "symlink-tar-member diagnostic: $(cat "$work/tamper-symlink-tar-member.log")"
run_tamper_case traversal-tar-member synthetic-fixture-token
grep -qF 'path traversal' "$work/tamper-traversal-tar-member.log" || fail "traversal-tar-member diagnostic: $(cat "$work/tamper-traversal-tar-member.log")"
run_tamper_case duplicate-tar-member synthetic-fixture-token
grep -qF 'duplicate tar entry' "$work/tamper-duplicate-tar-member.log" || fail "duplicate-tar-member diagnostic: $(cat "$work/tamper-duplicate-tar-member.log")"
if python3 "$fixture/tools/release/verify_handoff.py" --list-public-assets --tag invalid >/dev/null 2>&1; then
  fail "verify_handoff.py --list-public-assets accepted invalid tag"
fi

: >"$FIXTURE_MUTATIONS"
: >"$FIXTURE_FORBIDDEN_CALLS"
rc=0
PATH="$fixture/bin:$PATH" FIXTURE_FORBID_BUILD=1 FIXTURE_RESOLVE_DIGEST="sha256:$(printf '%064d' 0)" \
  GITHUB_TOKEN=synthetic-fixture-token \
  bash "$fixture/tools/release/publish.sh" example/template v0.0.1-fixture "$GOOD_HANDOFF" >"$work/resolve-mismatch.log" 2>&1 || rc=$?
want_mismatch_seq=$'oras-login\noras-cp\noras-resolve'
if [[ "$rc" != 1 || "$(cat "$FIXTURE_MUTATIONS")" != "$want_mismatch_seq" ]]; then
  fail "publish resolve-mismatch: rc=$rc mutations=$(tr '\n' ',' <"$FIXTURE_MUTATIONS")"
  cat "$work/resolve-mismatch.log" >&2
fi

# Suite 3: offline pin-verification controls for tools/release/oras.sh.
oras_tree="$work/oras-tree"
mkdir -p "$oras_tree/tools/release" "$work/oras-src" "$work/releases/v9.9.9"
sed -e "s|^releases=.*|releases=\"file://$work/releases\"|" \
  -e "s|--proto '=https' --tlsv1.2|--proto '=file'|" "$oras_src" >"$oras_tree/tools/release/oras.sh"
chmod +x "$oras_tree/tools/release/oras.sh"
cmp -s "$oras_src" "$oras_tree/tools/release/oras.sh" && fail "oras.sh release host replacement failed"
cp -- "$pins_src" "$oras_tree/tools/release/oras.pins"

cat >"$work/oras-src/oras" <<'STUB_ORAS'
#!/usr/bin/env bash
echo "args: $*"
echo "GITHUB_TOKEN=${GITHUB_TOKEN:-unset} GH_TOKEN=${GH_TOKEN:-unset}"
STUB_ORAS
chmod +x "$work/oras-src/oras"
oras_archive="$work/releases/v9.9.9/oras_9.9.9_linux_amd64.tar.gz"
tar -czf "$oras_archive" -C "$work/oras-src" oras
arch_sha="$(sha256sum "$oras_archive" | cut -d' ' -f1)"
bin_sha="$(sha256sum "$work/oras-src/oras" | cut -d' ' -f1)"
pin_oras() { printf '9.9.9 linux_amd64 %s %s\n' "$1" "$2" >"$oras_tree/tools/release/oras.pins"; }
run_oras() { XDG_CACHE_HOME="$work/oras-cache" "$oras_tree/tools/release/oras.sh" "$@"; }

pin_oras "$arch_sha" "$bin_sha"
if ! oras_out="$(GITHUB_TOKEN=leaked GH_TOKEN=leaked run_oras cp src dst 2>&1)"; then
  fail "oras.sh positive run failed: $oras_out"
else
  grep -qF 'args: cp src dst' <<<"$oras_out" || fail "oras.sh args missing: $oras_out"
  grep -qF 'GITHUB_TOKEN=unset GH_TOKEN=unset' <<<"$oras_out" || fail "oras.sh leaked token to binary: $oras_out"
fi
[[ "$(run_oras --path)" == "$work/oras-cache/bff-template/oras/9.9.9-linux_amd64-$bin_sha/oras" ]] || fail "oras.sh --path"

printf '#!/bin/sh\necho tampered\n' >"$(run_oras --path)"
if out="$(run_oras version 2>&1)"; then fail "tampered cached oras ran: $out"; fi
grep -qF 'checksum mismatch for cached' <<<"$out" || fail "tampered cache diagnostic: $out"

rm -rf "$work/oras-cache"
pin_oras "$(printf 'wrong' | sha256sum | cut -d' ' -f1)" "$bin_sha"
if out="$(run_oras version 2>&1)"; then fail "wrong oras archive checksum accepted: $out"; fi
grep -qF 'checksum mismatch for downloaded' <<<"$out" || fail "download checksum diagnostic: $out"
[[ -z "$(find "$work/oras-cache" -type f 2>/dev/null)" ]] || fail "rejected oras archive left cache file behind"

pin_oras "$arch_sha" "$bin_sha"
mkdir -p "$work/curlhome"
printf 'fail-with-body-this-option-does-not-exist\n' >"$work/curlhome/.curlrc"
HOME="$work/curlhome" run_oras version >/dev/null 2>&1 || fail "hostile ~/.curlrc broke oras.sh"

[[ "$failures" -eq 0 ]] && echo "ok credentialless prepare, handoff tamper checks, write-scoped ORAS publish boundary, and pinned ORAS checksums"
exit "$failures"
