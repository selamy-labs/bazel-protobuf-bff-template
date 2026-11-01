#!/usr/bin/env bash
# Witness for offline.sh (spec 012 FR-012): the declared service passes offline, then each planted defect fails
# against a copy: refused inputs, a third resource, a hard-coded project, an import block, a data source, an
# over-broad invoker role, a module, unformatted configuration. Copies dereference links.
#
#   offline_test.sh <offline.sh> <tofu> <provider zip> <service dir> <inputs file> <validator>
set -euo pipefail
script="$PWD/$1"; tofu="$PWD/$2"; zip="$PWD/$3"; service="$PWD/$4"; inputs="$PWD/$5"; validator="$PWD/$6"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
failures=0
replace() { local content; content="$(cat "$1")"; printf '%s\n' "${content/"$2"/"$3"}" >"$1"; }
expect() { # expect <name> <want> <mutation> [tofu]
  local name="$1" want="$2" rc=0
  rm -rf "$T/s"; cp -rL "$service" "$T/s"; cp -L "$inputs" "$T/inputs.json"
  case "$3" in
    none) ;;
    refused_inputs) replace "$T/inputs.json" '"allowed_origin": "https://example-owner.github.example.invalid"' '"allowed_origin": "https://*.example.invalid"' ;;
    serving_unknown_digest) replace "$T/inputs.json" '"serving_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"' '"serving_digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111"' ;;
    third_resource) printf '\nresource "google_storage_bucket" "extra" {\n  name     = "extra"\n  location = "US"\n}\n' >>"$T/s/main.tf" ;;
    hard_coded_project) replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  project = "some-real-project"' ;;
    import_block) printf '\nimport {\n  to = google_cloud_run_v2_service.bff\n  id = "existing"\n}\n' >>"$T/s/main.tf" ;;
    data_source) printf '\ndata "google_project" "p" {\n}\n' >>"$T/s/main.tf" ;;
    admin_invoker) replace "$T/s/main.tf" '  role     = "roles/run.invoker"' '  role     = "roles/run.admin"' ;;
    module_block) printf '\nmodule "extra" {\n  source = "./extra"\n}\n' >>"$T/s/main.tf"; mkdir -p "$T/s/extra"; printf 'resource "google_storage_bucket" "b" {\n  name     = "b"\n  location = "US"\n}\n' >"$T/s/extra/main.tf" ;;
    unformatted) replace "$T/s/main.tf" '  name     = "bff"' '  name="bff"' ;;
    *) echo "unknown mutation $3"; exit 2 ;;
  esac
  "$script" "${4:-$tofu}" "$zip" "$T/s" "$T/inputs.json" "$validator" >"$T/out" 2>&1 || rc=$?
  if [[ "$rc" == "$want" ]]; then echo "ok   $name"; else echo "FAIL $name: exit $rc"; tail -5 "$T/out" | sed 's/^/     /'; failures=$((failures + 1)); fi
}
expect "the declared service passes offline" 0 none
grep -q 'nothing applied' "$T/out" || { echo "FAIL the pass is not stated"; failures=$((failures + 1)); }
expect "refused inputs end it before any plan" 1 refused_inputs
grep -q 'no plan' "$T/out" || { echo "FAIL the refusal is not stated"; failures=$((failures + 1)); }
expect "a serving digest that is not the image's" 1 serving_unknown_digest
expect "a third resource" 1 third_resource
expect "a hard-coded project" 1 hard_coded_project
expect "an import block" 1 import_block
expect "a data source" 1 data_source
expect "an invoker role wider than invoke" 1 admin_invoker
expect "a module that adds a resource" 1 module_block
expect "unformatted configuration" 1 unformatted
expect "no tofu" 11 none "$T/absent"
expect "restored control" 0 none
exit "$failures"
