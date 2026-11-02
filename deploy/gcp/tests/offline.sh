#!/usr/bin/env bash
# The hosted example's service module as a reviewed plan (spec 012 FR-012): the inputs validated first, then
# format, validate, the pinned resource graph and the mock tests, with no reachable network and no credentials.
# The provider comes from a mirror assembled from the pinned zip and the lock's hash, so nothing is fetched.
# Nothing here applies anything.
#
#   offline.sh <tofu> <provider zip> <service dir> <inputs file> <validator> <rollback inputs> <cancel inputs> <pipeline>
#
# Exit 0 passed, 1 a finding, 11 nothing could be judged.
set -euo pipefail
tofu="$1"; zip="$2"; service="$3"; inputs="$4"; validator="$5"; rollback="$6"; cancel="$7"; pipeline="$8"
refuse() { echo "offline: $1" >&2; exit 11; }
[[ -x "$tofu" ]] || refuse "no tofu at $tofu"
[[ -f "$zip" ]] || refuse "no provider zip at $zip"
[[ -x "$validator" ]] || refuse "no inputs validator at $validator"
[[ -f "$rollback" && -f "$cancel" && -f "$pipeline" ]] || refuse "the rollback inputs, the cancel inputs or the pipeline is missing"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
# The one funnel first: a refusal here is a finding, before any plan. As in the pipeline, the plan reads the
# effective document the funnel wrote, never the raw file.
"$validator" --effective "$work/effective.inputs.json" "$inputs" >"$work/validate.out" || { echo "offline: the inputs are refused; no plan"; exit 1; }
[[ -s "$work/effective.inputs.json" ]] || { echo "offline: the funnel wrote no effective document"; exit 1; }
grep -q '^release.serving_digest="sha256:[0-9a-f]*" # from ' "$work/validate.out" || { echo "offline: the funnel named no source for the serving digest"; exit 1; }
hash="$(grep -oE 'h1:[A-Za-z0-9+/=]+' "$service/.terraform.lock.hcl" | head -1)"
[[ -n "$hash" ]] || refuse "the lock names no h1 hash"
mirror="$work/mirror/registry.opentofu.org/hashicorp/google"
mkdir -p "$mirror"
cp "$zip" "$mirror/terraform-provider-google_8.9.0_linux_amd64.zip"
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
mkdir -p "$work/deploy/gcp/inputs"
cp -rL "$service" "$work/deploy/gcp/service"
cp "$work/effective.inputs.json" "$work/deploy/gcp/inputs/example.inputs.json"
# The rollback and the cancellation inputs pass the same funnel before the mock tests read them (FR-009).
mkdir -p "$work/deploy/gcp/inputs/testdata"
"$validator" --effective "$work/deploy/gcp/inputs/testdata/rollback.inputs.json" "$rollback" >/dev/null || { echo "offline: the rollback inputs are refused"; exit 1; }
"$validator" --effective "$work/deploy/gcp/inputs/testdata/cancel.inputs.json" "$cancel" >"$work/cancel.out" || { echo "offline: the cancel inputs are refused"; exit 1; }
grep -q '^release.cancel_in_flight=true # from ' "$work/cancel.out" || { echo "offline: the funnel did not carry the cancellation"; exit 1; }
# The pipeline honors the cancellation: the cancel step reads the funnel's line, comes before anything is
# promoted, and every step that promotes, plans or applies stops at the marker.
grep -q "grep -q '^release.cancel_in_flight=true ' /workspace/validate.out" "$pipeline" || { echo "offline: the pipeline does not read the cancellation from the funnel"; exit 1; }
order="$(grep -oE '^  - id: [a-z-]+' "$pipeline" | sed 's/^  - id: //' | tr '\n' ' ')"
[[ "$order" == "validate-inputs cancel-in-flight promote-by-digest plan apply-behind-the-gate record " ]] || { echo "offline: the pipeline's steps are [$order], not the enumerated order"; exit 1; }
for step in promote-by-digest plan apply-behind-the-gate; do
  awk -v step="$step" '$0 ~ "^  - id: "step"$" {inside=1; next} inside && /^  - id: / {exit} inside && /\[\[ ! -e \/workspace\/cancelled \]\]/ {found=1} END {exit !found}' "$pipeline" || { echo "offline: the step $step ignores a cancellation"; exit 1; }
done
config="$work/deploy/gcp/service"
export HOME="$work/home" TF_PLUGIN_CACHE_DIR="" TF_CLI_CONFIG_FILE="$work/cli.tfrc" HTTPS_PROXY=http://127.0.0.1:9 HTTP_PROXY=http://127.0.0.1:9 NO_PROXY="" GOOGLE_APPLICATION_CREDENTIALS="" GOOGLE_CREDENTIALS="" CLOUDSDK_CONFIG="$work/home" TF_INPUT=0
mkdir -p "$HOME"
rm -rf "$config/.terraform"
"$tofu" -chdir="$config" fmt -check -recursive >/dev/null || { echo "offline: the configuration is not formatted"; exit 1; }
# The declaration is read structurally: no JSON configuration, no module, no import, no data source, exactly
# the two resources, and then the real graph that tofu builds from the loaded configuration.
if find "$config" -name '*.tf.json' -o -name '*.tofu.json' | grep -q .; then echo "offline: JSON configuration is not accepted here"; exit 1; fi
if grep -qE '^\s*(module|import|data)\s' "$config"/*.tf; then echo "offline: a module, import or data source is not accepted here"; exit 1; fi
[[ "$(cat "$config"/*.tf | grep -cE '^resource ')" == 2 ]] || { echo "offline: the resource graph is not the two declared resources"; exit 1; }
"$tofu" -chdir="$config" init -input=false -lockfile=readonly >/dev/null || refuse "init did not complete from the mirror"
"$tofu" -chdir="$config" validate >/dev/null || { echo "offline: the configuration does not validate"; exit 1; }
graph="$("$tofu" -chdir="$config" graph 2>/dev/null)" || refuse "the graph could not be built"
# The graph names each resource as "[root] <address> (expand)"; an empty extraction is a finding, not a silent end.
nodes="$(grep -oE '"(\[root\] )?google_[a-z0-9_]+\.[A-Za-z0-9_-]+( \(expand\))?"' <<<"$graph" | sed -e 's/\[root\] //' -e 's/ (expand)//' | LC_ALL=C sort -u | tr -d '"' | tr '\n' ' ' || true)"
[[ "$nodes" == "google_cloud_run_v2_service.bff google_cloud_run_v2_service_iam_member.public_invoker " ]] || { echo "offline: the loaded graph holds [$nodes], not the two declared resources"; exit 1; }
# The provider must be bound to the funnel's project and region, never a literal.
grep -qE '^\s*project\s*=\s*jsondecode\(file\(var\.inputs_file\)\)\.contract\.project_id\s*$' "$config/versions.tf" || { echo "offline: the provider is not bound to the funnel's project"; exit 1; }
grep -qE '^\s*region\s*=\s*jsondecode\(file\(var\.inputs_file\)\)\.contract\.region\s*$' "$config/versions.tf" || { echo "offline: the provider is not bound to the funnel's region"; exit 1; }
"$tofu" -chdir="$config" test -no-color -var "inputs_file=../inputs/example.inputs.json" >"$work/test.out" 2>&1 || { cat "$work/test.out"; echo "offline: a mock test failed"; exit 1; }
grep -q "6 passed, 0 failed" "$work/test.out" || { cat "$work/test.out"; echo "offline: not every mock test ran"; exit 1; }
echo "offline: the service module validates and its mock tests pass offline against the synthetic inputs; nothing applied"
