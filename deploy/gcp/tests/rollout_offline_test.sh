#!/usr/bin/env bash
# Offline hermetic verification for issue #20 GCP rollout modules (foundation, release, cd, smoke):
# runs pinned OpenTofu fmt/init/validate/test from the pinned provider mirror with zero network,
# verifies default foundation -> inputs:validate -> service/main.tf composition, and runs all
# release/cd/smoke unit tests with no bytecode artifacts. Nothing here applies anything.
#
#   rollout_offline_test.sh <tofu> <provider zip> <foundation dir> <cd dir> <release dir> <smoke dir> <verify_handoff.py> <inputs validator> <service main.tf> <same-origin inputs>
set -euo pipefail

tofu="$PWD/$1"
zip="$PWD/$2"
foundation_dir="$PWD/$3"
cd_dir="$PWD/$4"
release_dir="$PWD/$5"
smoke_dir="$PWD/$6"
verify_handoff="$PWD/$7"
inputs_validator="$PWD/$8"
service_main_tf="$PWD/$9"
same_origin_inputs="$PWD/${10}"

[[ -x "$tofu" ]] || { echo "rollout-offline: missing tofu at $tofu" >&2; exit 11; }
[[ -f "$zip" ]] || { echo "rollout-offline: missing provider zip at $zip" >&2; exit 11; }
[[ -f "$verify_handoff" ]] || { echo "rollout-offline: missing verify_handoff.py at $verify_handoff" >&2; exit 11; }
[[ -x "$inputs_validator" ]] || { echo "rollout-offline: missing inputs validator at $inputs_validator" >&2; exit 11; }
[[ -f "$service_main_tf" ]] || { echo "rollout-offline: missing service main.tf at $service_main_tf" >&2; exit 11; }
[[ -f "$same_origin_inputs" ]] || { echo "rollout-offline: missing same-origin inputs at $same_origin_inputs" >&2; exit 11; }

work="$(mktemp -d)"
trap 'chmod -R u+w "$work" 2>/dev/null || true; rm -rf -- "$work"' EXIT

hash="$(grep -oE 'h1:[A-Za-z0-9+/=]+' "$foundation_dir/.terraform.lock.hcl" | sed -n '1p')"
[[ -n "$hash" ]] || { echo "rollout-offline: missing provider h1 hash in lockfile" >&2; exit 11; }

mirror="$work/mirror/registry.opentofu.org/hashicorp/google"
mkdir -p "$mirror" "$work/home"
cp -- "$zip" "$mirror/terraform-provider-google_8.9.0_linux_amd64.zip"
printf '{"versions":{"8.9.0":{}}}\n' >"$mirror/index.json"
printf '{"archives":{"linux_amd64":{"url":"terraform-provider-google_8.9.0_linux_amd64.zip","hashes":["%s"]}}}\n' "$hash" >"$mirror/8.9.0.json"

cat >"$work/cli.tfrc" <<CLI
provider_installation {
  filesystem_mirror {
    path    = "$work/mirror"
    include = ["hashicorp/google"]
  }
}
CLI

export HOME="$work/home"
export TF_PLUGIN_CACHE_DIR=""
export TF_CLI_CONFIG_FILE="$work/cli.tfrc"
export HTTPS_PROXY="http://127.0.0.1:9"
export HTTP_PROXY="http://127.0.0.1:9"
export NO_PROXY=""
export GOOGLE_APPLICATION_CREDENTIALS=""
export GOOGLE_CREDENTIALS=""
export CLOUDSDK_CONFIG="$work/home"
export TF_INPUT=0
export PYTHONDONTWRITEBYTECODE=1

for pair in "foundation:$foundation_dir" "cd:$cd_dir"; do
  name="${pair%%:*}"
  src="${pair#*:}"
  mod="$work/$name"
  cp -rL -- "$src" "$mod"
  rm -rf -- "$mod/.terraform"
  "$tofu" -chdir="$mod" fmt -check -recursive -no-color >/dev/null
  "$tofu" -chdir="$mod" init -backend=false -input=false -lockfile=readonly -no-color >/dev/null
  "$tofu" -chdir="$mod" validate -no-color >/dev/null
  "$tofu" -chdir="$mod" test -no-color
done

# Composition control: verify default foundation dev_service_name and contract_v1 compose directly
# with deploy/gcp/service/main.tf and deploy/gcp/inputs:validate.
python3 -B - "$foundation_dir/variables.tf" "$service_main_tf" "$same_origin_inputs" "$inputs_validator" "$work/composed.inputs.json" "$work/effective.inputs.json" <<'PY'
import json, re, subprocess, sys
from pathlib import Path

vars_tf = Path(sys.argv[1]).read_text(encoding="utf-8")
service_tf = Path(sys.argv[2]).read_text(encoding="utf-8")
base_inputs = json.loads(Path(sys.argv[3]).read_text(encoding="utf-8"))
validator, raw_inputs, eff_inputs = sys.argv[4], Path(sys.argv[5]), Path(sys.argv[6])

dev_match = re.search(r'variable\s+"dev_service_name"\s*\{[^}]*default\s*=\s*"([^"]+)"', vars_tf, re.S)
svc_match = re.search(r'resource\s+"google_cloud_run_v2_service"\s+"bff"\s*\{[^}]*\bname\s*=\s*"([^"]+)"', service_tf, re.S)
assert dev_match and svc_match, "could not parse dev_service_name or service name"
if dev_match.group(1) != svc_match.group(1):
    raise SystemExit(f"foundation default dev_service_name {dev_match.group(1)!r} != service name {svc_match.group(1)!r}")

project_id = "example-bff-hosted-000000"
region = "example-region1"
base_inputs["region"] = region
base_inputs["contract"] = {
    "version": 1,
    "project_id": project_id,
    "region": region,
    "runtime_identity": "bff-runtime@example.invalid",
    "build_identity": "bff-dev-deployer@example.invalid",
    "artifact_registry": f"{region}-docker.pkg.dev/{project_id}/bff-images/bff",
    "state_bucket": f"{project_id}-state",
    "notes_bucket": f"{project_id}-notes",
}
raw_inputs.write_text(json.dumps(base_inputs, indent=2) + "\n", encoding="utf-8")
subprocess.run([validator, "--effective", str(eff_inputs), str(raw_inputs)], check=True, stdout=subprocess.DEVNULL)
PY

cp -L -- "$verify_handoff" "$work/verify_handoff.py"
cp -rL -- "$release_dir" "$work/release"

for suite in "$work/release/tests" "$cd_dir/tests" "$smoke_dir/tests"; do
  NO_PROXY="127.0.0.1,localhost" no_proxy="127.0.0.1,localhost" \
    RELEASE_VERIFY_HANDOFF_SCRIPT="$work/verify_handoff.py" \
    python3 -B -m unittest discover -s "$suite" -p "test_*.py"
done

echo "rollout-offline: foundation, cd, release, smoke, and default-contract composition verified offline; nothing applied"
