#!/usr/bin/env bash
# Witness for offline.sh (spec 012 FR-012): the declared service passes offline, then each planted defect fails
# against a copy: refused inputs, a third resource, a hard-coded project, an import block, a data source, an
# over-broad invoker role, a module, unformatted configuration. Copies dereference links.
#
#   offline_test.sh <offline.sh> <tofu> <provider zip> <service dir> <inputs file> <validator> <rollback inputs> <cancel inputs> <pipeline> <yq>
set -euo pipefail
script="$PWD/$1"; tofu="$PWD/$2"; zip="$PWD/$3"; service="$PWD/$4"; inputs="$PWD/$5"; validator="$PWD/$6"; rollback="$PWD/$7"; cancel="$PWD/$8"; pipeline="$PWD/$9"; yq="$PWD/${10}"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
failures=0
replace() { local content; content="$(cat "$1")"; printf '%s\n' "${content/"$2"/"$3"}" >"$1"; }
expect() { # expect <name> <want> <mutation> [tofu]
  local name="$1" want="$2" rc=0
  rm -rf "$T/s"; cp -rL "$service" "$T/s"; cp -L "$inputs" "$T/inputs.json"; cp -L "$pipeline" "$T/cloudbuild.yaml"
  # shellcheck disable=SC2016
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
    rollback_changes_more) replace "$T/s/main.tf" '      max_instance_count = local.inputs.bounds.max_instances' '      max_instance_count = local.inputs.release.previous_digest == null ? local.inputs.bounds.max_instances : 1' ;;
    plain_http_telemetry) replace "$T/inputs.json" '"endpoint": "https://otel.example.invalid"' '"endpoint": "http://otel.example.invalid"' ;;
    null_sample_ratio) replace "$T/inputs.json" '"sample_ratio": 1' '"sample_ratio": null' ;;
    telemetry_with_no_host) replace "$T/inputs.json" '"endpoint": "https://otel.example.invalid"' '"endpoint": "https://:443"' ;;
    loopback_telemetry) replace "$T/inputs.json" '"endpoint": "https://otel.example.invalid"' '"endpoint": "https://127.0.0.1:4318"' ;;
    telemetry_endpoint_dropped) replace "$T/s/main.tf" 'name  = "BFF_OTEL_ENDPOINT"' 'name  = "BFF_OTEL_ENDPOINT_UNREAD"' ;;
    rollback_reports_the_newer_revision) replace "$T/s/main.tf" ': local.inputs.release.previous_revision' ': local.inputs.image.source_revision' ;;
    cancel_step_removed) replace "$T/cloudbuild.yaml" "grep -q '^release.cancel_in_flight=true ' /workspace/validate.out" "false" ;;
    apply_ignores_cancel) replace "$T/cloudbuild.yaml" '        [[ ! -e /workspace/cancelled ]] || { echo "skipped: cancelled"; exit 0; }
        # The gate' '        # The gate' ;;
    count_zero_extra_binding) printf '\nresource "google_cloud_run_v2_service_iam_member" "extra" {\n  count    = 0\n  project  = google_cloud_run_v2_service.bff.project\n  location = google_cloud_run_v2_service.bff.location\n  name     = google_cloud_run_v2_service.bff.name\n  role     = "roles/owner"\n  member   = "allUsers"\n}\n' >>"$T/s/main.tf" ;;
    provider_project_wrong_field) replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  project = jsondecode(file(var.inputs_file)).contract.notes_bucket' ;;
    provider_region_wrong_field) replace "$T/s/versions.tf" '  region  = jsondecode(file(var.inputs_file)).contract.region' '  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket' ;;
    pipefail_removed) replace "$T/cloudbuild.yaml" '        set -o pipefail' '        echo "omitted"' ;;
    digest_check_removed) replace "$T/cloudbuild.yaml" "printf '%s  /workspace/plan.tfplan\n' \"\${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -" 'echo "omitted"' ;;
    literal_concurrency) replace "$T/s/main.tf" 'max_instance_request_concurrency = local.inputs.bounds.concurrency' 'max_instance_request_concurrency = 20' ;;
    literal_timeout) replace "$T/s/main.tf" 'timeout                          = "${local.inputs.bounds.request_timeout_seconds}s"' 'timeout                          = "30s"' ;;
    literal_max_instances) replace "$T/s/main.tf" 'max_instance_count = local.inputs.bounds.max_instances' 'max_instance_count = 2' ;;
    budget_output_in_other_file) printf '\noutput "owner_budget" {\n  value = local.inputs.bounds.budget_alert_amount\n}\n' >>"$T/s/extra.tf" ;;
    pipefail_comment_only) replace "$T/cloudbuild.yaml" 'set -o pipefail' '# set -o pipefail' ;;
    digest_check_comment_only) replace "$T/cloudbuild.yaml" "printf '%s  /workspace/plan.tfplan\n' \"\${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -" '# printf checksum' ;;
    digest_checked_after_apply)
      replace "$T/cloudbuild.yaml" "printf '%s  /workspace/plan.tfplan\n' \"\${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -" '# moved after apply'
      replace "$T/cloudbuild.yaml" 'tofu apply -input=false -auto-approve /workspace/plan.tfplan' 'tofu apply -input=false -auto-approve /workspace/plan.tfplan
        printf '\''%s  /workspace/plan.tfplan\n'\'' "${_APPROVED_PLAN_SHA256}" | sha256sum --strict -c -'
      ;;
    owner_checksum_removed)
      replace "$T/cloudbuild.yaml" "printf '%s  /workspace/plan.tfplan\n' \"\${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -" '# checksum check removed'
      ;;
    apply_runs_without_approval)
      replace "$T/cloudbuild.yaml" 'if [[ -z "${_APPROVED_PLAN_SHA256:-}" ]]; then
          echo "plan mode: _APPROVED_PLAN_SHA256 is empty; skipping apply behind approval gate"
          exit 0
        fi' '# plan mode check omitted'
      ;;
    pipefail_printed) replace "$T/cloudbuild.yaml" 'set -o pipefail' 'echo "set -o pipefail"' ;;
    cancel_guard_printed) replace "$T/cloudbuild.yaml" '[[ ! -e /workspace/cancelled ]] || { echo "skipped: cancelled"; exit 0; }' 'echo "[[ ! -e /workspace/cancelled ]]"' ;;
    budget_alias_output)
      printf '\nlocals {\n  owner_limit = local.inputs.bounds.budget_alert_amount\n}\noutput "owner_budget" {\n  value = local.owner_limit\n}\n' >>"$T/s/extra.tf"
      ;;
    provider_nested_mask)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  default_labels = {
    project = jsondecode(file(var.inputs_file)).contract.project_id
    region  = jsondecode(file(var.inputs_file)).contract.region
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket'
      replace "$T/s/versions.tf" '  region  = jsondecode(file(var.inputs_file)).contract.region' '  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket'
      ;;
    tofu_file_present) printf '\nresource "google_storage_bucket" "b" {\n  name     = "b"\n  location = "US"\n}\n' >"$T/s/extra.tofu" ;;
    override_file_present) printf '\nprovider "google" {\n  project = "extra-proj"\n}\n' >"$T/s/versions_override.tf" ;;
    harmless_block_comment) printf '\n/* Harmless block comment */\n' >>"$T/s/main.tf" ;;
    harmless_heredoc)
      printf 'locals {\n  note = <<-EOT\n  import {\n    this = "heredoc"\n  }\n  EOT\n}\n' >"$T/s/extra.tf"
      ;;
    harmless_quoted_step_ids) replace "$T/cloudbuild.yaml" '- id: plan' '- id: "plan"' ;;
    provider_local_expression)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  project = local.inputs.contract.project_id'
      replace "$T/s/versions.tf" '  region  = jsondecode(file(var.inputs_file)).contract.region' '  region  = local.inputs.contract.region'
      ;;
    harmless_provider_heredoc)
      printf 'locals {\n  note = <<-EOT\n  provider "aws" {}\n  EOT\n}\n' >"$T/s/extra.tf"
      ;;
    budget_alias_in_serving)
      replace "$T/s/outputs.tf" '    revision       = local.serving_revision' '    revision       = local.serving_revision
    owner_budget   = local.inputs.bounds.budget_alert_amount'
      ;;
    budget_changes_resource_bound)
      replace "$T/s/main.tf" 'max_instance_count = local.inputs.bounds.max_instances' 'max_instance_count = local.inputs.bounds.budget_alert_amount == 100 ? 10 : local.inputs.bounds.max_instances'
      ;;
    budget_run_deleted)
      sed -i '/# The serving output is invariant under budget alert changes/,$d' "$T/s/tests/service.tftest.hcl"
      sed -i '${/^$/d;}' "$T/s/tests/service.tftest.hcl"
      ;;
    brace_nested_mask)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  default_labels = {
    mask    = "}"
    project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  }'
      ;;
    brace_harmless)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  default_labels = {
    note = "{"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id'
      ;;
    plan_generation_deleted)
      replace "$T/cloudbuild.yaml" 'tofu plan -input=false -var "inputs_file=/workspace/effective.inputs.json" -out=/workspace/plan.tfplan' '# tofu plan deleted'
      ;;
    storage_upload_deleted)
      replace "$T/cloudbuild.yaml" 'gcloud storage cp /workspace/plan.tfplan "${storage_prefix}/plan.tfplan"' '# storage cp plan deleted'
      ;;
    approval_regex_deleted)
      content="$(cat "$T/cloudbuild.yaml")"
      target='if [[ ! "${_APPROVED_PLAN_SHA256}" =~ ^[0-9a-f]{64}$ ]]; then'
      printf '%s\n' "${content//"$target"/"if false; then"}" >"$T/cloudbuild.yaml"
      ;;
    comment_quote)
      printf '# An operator wrote " here as documentation.\n' | cat - "$T/s/versions.tf" > "$T/s/v.tmp" && mv "$T/s/v.tmp" "$T/s/versions.tf"
      ;;
    comment_double_slash_quote)
      printf '// Documentation with unmatched " quote\n' | cat - "$T/s/versions.tf" > "$T/s/v.tmp" && mv "$T/s/v.tmp" "$T/s/versions.tf"
      ;;
    comment_block_quote)
      printf '/* Block comment with unmatched " quote and { braces } */\n' | cat - "$T/s/versions.tf" > "$T/s/v.tmp" && mv "$T/s/v.tmp" "$T/s/versions.tf"
      ;;
    apply_replans_before_apply)
      replace "$T/cloudbuild.yaml" 'tofu apply -input=false -auto-approve /workspace/plan.tfplan' 'tofu plan -input=false -out=/workspace/plan.tfplan
        tofu apply -input=false -auto-approve /workspace/plan.tfplan'
      ;;
    apply_extra_flags)
      replace "$T/cloudbuild.yaml" 'tofu apply -input=false -auto-approve /workspace/plan.tfplan' 'tofu apply -var "env=prod" -input=false -auto-approve /workspace/plan.tfplan'
      ;;
    apply_wrong_suffix_path)
      replace "$T/cloudbuild.yaml" 'tofu apply -input=false -auto-approve /workspace/plan.tfplan' 'tofu apply -input=false -auto-approve /workspace/plan.tfplan.unapproved'
      ;;
    crane_digest_deleted)
      replace "$T/cloudbuild.yaml" 'test "$(crane digest "${_ARTIFACT_REGISTRY}@${_SERVING_DIGEST}")" = "${_SERVING_DIGEST}"' '# crane digest deleted'
      ;;
    cancel_self_filter_deleted)
      replace "$T/cloudbuild.yaml" ' AND id!=${BUILD_ID}' ''
      ;;
    record_describe_deleted)
      replace "$T/cloudbuild.yaml" 'gcloud run services describe bff --region "${_REGION}" --format=json > /workspace/service.after.json' '# record describe deleted'
      ;;
    apply_global_option_replan)
      replace "$T/cloudbuild.yaml" 'tofu apply -input=false -auto-approve /workspace/plan.tfplan' 'tofu -chdir=. plan -input=false -out=/workspace/./plan.tfplan
        tofu apply -input=false -auto-approve /workspace/plan.tfplan'
      ;;
    apply_artifact_replace_restore)
      replace "$T/cloudbuild.yaml" 'tofu apply -input=false -auto-approve /workspace/plan.tfplan' 'cp /workspace/plan.tfplan /workspace/plan.orig
        echo "tampered-unapproved-plan-bytes" > /workspace/plan.tfplan
        export _APPROVED_PLAN_SHA256="$(sha256sum /workspace/plan.tfplan | awk '\''{print $1}'\'')"
        tofu apply -input=false -auto-approve /workspace/plan.tfplan
        cp /workspace/plan.orig /workspace/plan.tfplan'
      ;;
    provider_interpolation_wrong_binding)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  default_labels = {
    mask    = "${"}"}"
    project = jsondecode(file(var.inputs_file)).contract.project_id
    region  = jsondecode(file(var.inputs_file)).contract.region
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket'
      replace "$T/s/versions.tf" '  region  = jsondecode(file(var.inputs_file)).contract.region' '  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket'
      ;;
    provider_interpolation_harmless)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  default_labels = {
    env = "${"dev"}"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id'
      ;;
    apply_tofu_destroy)
      replace "$T/cloudbuild.yaml" 'tofu apply -input=false -auto-approve /workspace/plan.tfplan' 'tofu destroy -auto-approve -input=false
        tofu apply -input=false -auto-approve /workspace/plan.tfplan'
      ;;
    apply_tofu_state_rm)
      replace "$T/cloudbuild.yaml" 'tofu apply -input=false -auto-approve /workspace/plan.tfplan' 'tofu state rm google_cloud_run_v2_service.bff
        tofu apply -input=false -auto-approve /workspace/plan.tfplan'
      ;;
    apply_tofu_import)
      replace "$T/cloudbuild.yaml" 'tofu apply -input=false -auto-approve /workspace/plan.tfplan' 'tofu import google_cloud_run_v2_service.bff x
        tofu apply -input=false -auto-approve /workspace/plan.tfplan'
      ;;
    apply_duplicate_apply)
      replace "$T/cloudbuild.yaml" 'tofu apply -input=false -auto-approve /workspace/plan.tfplan' 'tofu apply -input=false -auto-approve /workspace/plan.tfplan
        tofu apply -input=false -auto-approve /workspace/plan.tfplan'
      ;;
    apply_tf_cli_args)
      replace "$T/cloudbuild.yaml" 'tofu apply -input=false -auto-approve /workspace/plan.tfplan' 'export TF_CLI_ARGS_apply=-lock=false
        tofu apply -input=false -auto-approve /workspace/plan.tfplan'
      ;;
    provider_nested_heredoc_wrong_binding)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  default_labels = {
    mask = "${trimspace(<<EOT
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
EOT
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket'
      replace "$T/s/versions.tf" '  region  = jsondecode(file(var.inputs_file)).contract.region' '  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket'
      ;;
    provider_nested_heredoc_unicode_wrong_binding)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  default_labels = {
    mask = "${trimspace(<<ÉOT
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
ÉOT
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket'
      replace "$T/s/versions.tf" '  region  = jsondecode(file(var.inputs_file)).contract.region' '  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket'
      ;;
    provider_nested_heredoc_harmless)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  default_labels = {
    note = "${chomp(<<-EOT
      }"
    EOT
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id'
      ;;
    provider_nested_heredoc_hyphen_wrong_binding)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  default_labels = {
    mask = "${trimspace(<<EOF-REAL
EOF
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
EOF-REAL
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket'
      replace "$T/s/versions.tf" '  region  = jsondecode(file(var.inputs_file)).contract.region' '  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket'
      ;;
    provider_nested_heredoc_hyphen_control)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  default_labels = {
    mask = "${trimspace(<<EOF-REAL
EOF
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
EOF-REAL
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id'
      ;;
    provider_nested_heredoc_combining_wrong_binding)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  default_labels = {
    mask = "${trimspace(<<ÉOT
E
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
ÉOT
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket'
      replace "$T/s/versions.tf" '  region  = jsondecode(file(var.inputs_file)).contract.region' '  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket'
      ;;
    provider_nested_heredoc_combining_control)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  default_labels = {
    mask = "${trimspace(<<ÉOT
E
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
ÉOT
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id'
      ;;
    provider_nested_heredoc_other_id_start_wrong_binding)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  default_labels = {
    mask = "${trimspace(<<℘
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
℘
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket'
      replace "$T/s/versions.tf" '  region  = jsondecode(file(var.inputs_file)).contract.region' '  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket'
      ;;
    provider_nested_heredoc_other_id_start_control)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id' '  default_labels = {
    mask = "${trimspace(<<℘
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
℘
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id'
      ;;
    provider_nested_heredoc_template_wrong_binding)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region' '  default_labels = {
    mask = "${trimspace(<<OUTER
${trimspace(<<INNER
OUTER
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
INNER
    )}
OUTER
)}"
}
project = jsondecode(file(var.inputs_file)).contract.notes_bucket
region  = jsondecode(file(var.inputs_file)).contract.notes_bucket'
      ;;
    provider_nested_heredoc_template_control)
      replace "$T/s/versions.tf" '  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region' '  default_labels = {
    mask = "${trimspace(<<OUTER
${trimspace(<<INNER
OUTER
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
INNER
    )}
OUTER
)}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region  = jsondecode(file(var.inputs_file)).contract.region'
      ;;
    apply_env_tf_workspace)
      replace "$T/cloudbuild.yaml" '  - id: apply-behind-the-gate
    name: "${_TOOLS_IMAGE}"
    entrypoint: bash
    env:
      - "_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}"
      - "_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}"' '  - id: apply-behind-the-gate
    name: "${_TOOLS_IMAGE}"
    entrypoint: bash
    env:
      - "_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}"
      - "_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}"
      - "TF_WORKSPACE=unreviewed"'
      ;;
    apply_env_tf_cli_config)
      replace "$T/cloudbuild.yaml" '  - id: apply-behind-the-gate
    name: "${_TOOLS_IMAGE}"
    entrypoint: bash
    env:
      - "_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}"
      - "_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}"' '  - id: apply-behind-the-gate
    name: "${_TOOLS_IMAGE}"
    entrypoint: bash
    env:
      - "_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}"
      - "_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}"
      - "TF_CLI_CONFIG_FILE=/workspace/unreviewed.tfrc"'
      ;;
    apply_env_opentofu_log)
      replace "$T/cloudbuild.yaml" '  - id: apply-behind-the-gate
    name: "${_TOOLS_IMAGE}"
    entrypoint: bash
    env:
      - "_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}"
      - "_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}"' '  - id: apply-behind-the-gate
    name: "${_TOOLS_IMAGE}"
    entrypoint: bash
    env:
      - "_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}"
      - "_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}"
      - "OPENTOFU_LOG=trace"'
      ;;
    global_options_env_tf_workspace)
      replace "$T/cloudbuild.yaml" 'options:
  logging: CLOUD_LOGGING_ONLY' 'options:
  logging: CLOUD_LOGGING_ONLY
  env: ["TF_WORKSPACE=unreviewed"]'
      ;;
    apply_env_neutral_control)
      replace "$T/cloudbuild.yaml" '  - id: apply-behind-the-gate
    name: "${_TOOLS_IMAGE}"
    entrypoint: bash
    env:
      - "_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}"
      - "_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}"' '  - id: apply-behind-the-gate
    name: "${_TOOLS_IMAGE}"
    entrypoint: bash
    env:
      - "_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}"
      - "_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}"
      - "MY_VAR=allowed"'
      ;;
    digest_check_masked_true)
      replace "$T/cloudbuild.yaml" "printf '%s  /workspace/plan.tfplan\n' \"\${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -" "printf '%s  /workspace/plan.tfplan\n' \"\${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c - || true"
      ;;
    sha256sum_strict_dropped)
      replace "$T/cloudbuild.yaml" 'sha256sum --strict -c -' 'sha256sum -c -'
      ;;
    sidecar_grep_removed)
      replace "$T/cloudbuild.yaml" 'grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256 || {' 'true || {'
      ;;
    *) echo "unknown mutation $3"; exit 2 ;;
  esac
  "$script" "${4:-$tofu}" "$zip" "$T/s" "$T/inputs.json" "$validator" "$rollback" "$cancel" "$T/cloudbuild.yaml" "$yq" >"$T/out" 2>&1 || rc=$?
  if [[ "$rc" == "$want" ]]; then echo "ok   $name"; else echo "FAIL $name: exit $rc"; tail -5 "$T/out" | sed 's/^/     /'; failures=$((failures + 1)); fi
}
expect "the declared service passes offline" 0 none
grep -q 'nothing applied' "$T/out" || { echo "FAIL the pass is not stated"; failures=$((failures + 1)); }
expect "refused inputs end it before any plan" 1 refused_inputs
grep -q 'no plan' "$T/out" || { echo "FAIL the refusal is not stated"; failures=$((failures + 1)); }
expect "a serving digest that is not the image's" 1 serving_unknown_digest
expect "a third resource" 1 third_resource
expect "a resource hidden behind count zero" 1 count_zero_extra_binding
expect "a hard-coded project" 1 hard_coded_project
expect "a provider project bound to the wrong field" 1 provider_project_wrong_field
expect "a provider region bound to the wrong field" 1 provider_region_wrong_field
expect "an import block" 1 import_block
expect "a data source" 1 data_source
expect "an invoker role wider than invoke" 1 admin_invoker
expect "a module that adds a resource" 1 module_block
expect "unformatted configuration" 1 unformatted
expect "a rollback that changes more than the digest and its revision" 1 rollback_changes_more
expect "a plain http telemetry endpoint" 1 plain_http_telemetry
expect "a null sample ratio" 1 null_sample_ratio
expect "a telemetry endpoint with no host" 1 telemetry_with_no_host
expect "a loopback telemetry endpoint" 1 loopback_telemetry
expect "a required telemetry input not passed" 1 telemetry_endpoint_dropped
expect "a rollback that reports the newer image's revision" 1 rollback_reports_the_newer_revision
expect "a pipeline that does not read the cancellation" 1 cancel_step_removed
expect "an apply step that ignores a cancellation" 1 apply_ignores_cancel
expect "a pipeline step without pipefail" 1 pipefail_removed
expect "an apply step that does not verify the plan digest" 1 digest_check_removed
expect "literal concurrency instead of inputs bounds" 1 literal_concurrency
expect "literal timeout instead of inputs bounds" 1 literal_timeout
expect "literal max instances instead of inputs bounds" 1 literal_max_instances
expect "budget alert consumed by output in another file" 1 budget_output_in_other_file
expect "pipefail only in a comment" 1 pipefail_comment_only
expect "digest check only in a comment" 1 digest_check_comment_only
expect "digest checked after apply" 1 digest_checked_after_apply
expect "owner plan checksum check removed" 1 owner_checksum_removed
expect "apply step runs without plan approval gate" 1 apply_runs_without_approval
expect "pipefail only printed" 1 pipefail_printed
expect "cancel guard only printed" 1 cancel_guard_printed
expect "budget alert consumed via local alias output" 1 budget_alias_output
expect "provider project masked by nested default_labels" 1 provider_nested_mask
expect "unauthorized .tofu configuration file" 1 tofu_file_present
expect "unauthorized override configuration file" 1 override_file_present
expect "harmless block comment" 0 harmless_block_comment
expect "harmless heredoc with import token" 0 harmless_heredoc
expect "harmless quoted step ids in pipeline" 0 harmless_quoted_step_ids
expect "equivalent local provider reference passes" 0 provider_local_expression
expect "harmless provider block inside heredoc passes" 0 harmless_provider_heredoc
expect "budget alert smuggled into serving output" 1 budget_alias_in_serving
expect "provider project masked by string brace in default_labels" 1 brace_nested_mask
expect "harmless string brace in default_labels" 0 brace_harmless
expect "plan generation deleted from pipeline" 1 plan_generation_deleted
expect "storage upload deleted from pipeline" 1 storage_upload_deleted
expect "approval regex validation deleted from pipeline" 1 approval_regex_deleted
expect "harmless hash comment with unmatched quote" 0 comment_quote
expect "harmless double slash comment with unmatched quote" 0 comment_double_slash_quote
expect "harmless block comment with unmatched quote" 0 comment_block_quote
expect "re-plan planted inside apply step" 1 apply_replans_before_apply
expect "extra flags in tofu apply" 1 apply_extra_flags
expect "apply with wrong plan path" 1 apply_wrong_suffix_path
expect "crane digest verification deleted" 1 crane_digest_deleted
expect "cancel current-build exclusion deleted" 1 cancel_self_filter_deleted
expect "record service-describe deleted" 1 record_describe_deleted
expect "global option replan planted inside apply step" 1 apply_global_option_replan
expect "artifact replace and restore during apply step" 1 apply_artifact_replace_restore
expect "provider project masked by string interpolation in default_labels" 1 provider_interpolation_wrong_binding
expect "harmless string interpolation in default_labels" 0 provider_interpolation_harmless
expect "tofu destroy planted inside apply step" 1 apply_tofu_destroy
expect "tofu state rm planted inside apply step" 1 apply_tofu_state_rm
expect "tofu import planted inside apply step" 1 apply_tofu_import
expect "duplicate tofu apply planted inside apply step" 1 apply_duplicate_apply
expect "TF_CLI_ARGS exported inside apply step" 1 apply_tf_cli_args
expect "TF_WORKSPACE in apply step env array" 1 apply_env_tf_workspace
expect "TF_CLI_CONFIG_FILE in apply step env array" 1 apply_env_tf_cli_config
expect "OPENTOFU_LOG in apply step env array" 1 apply_env_opentofu_log
expect "TF_WORKSPACE in options env array" 1 global_options_env_tf_workspace
expect "permitted neutral environment variable in apply step" 0 apply_env_neutral_control
expect "approved digest check masked with || true" 1 digest_check_masked_true
expect "sha256sum without --strict flag" 1 sha256sum_strict_dropped
expect "sidecar checksum grep removed" 1 sidecar_grep_removed
expect "provider project masked by nested heredoc in default_labels" 1 provider_nested_heredoc_wrong_binding
expect "provider project masked by unicode nested heredoc in default_labels" 1 provider_nested_heredoc_unicode_wrong_binding
expect "harmless nested heredoc in default_labels" 0 provider_nested_heredoc_harmless
expect "provider project masked by hyphenated heredoc delimiter in default_labels" 1 provider_nested_heredoc_hyphen_wrong_binding
expect "harmless hyphenated heredoc delimiter in default_labels" 0 provider_nested_heredoc_hyphen_control
expect "provider project masked by combining mark heredoc delimiter in default_labels" 1 provider_nested_heredoc_combining_wrong_binding
expect "harmless combining mark heredoc delimiter in default_labels" 0 provider_nested_heredoc_combining_control
expect "provider project masked by other id start heredoc delimiter in default_labels" 1 provider_nested_heredoc_other_id_start_wrong_binding
expect "harmless other id start heredoc delimiter in default_labels" 0 provider_nested_heredoc_other_id_start_control
expect "provider project masked by nested heredoc template in default_labels" 1 provider_nested_heredoc_template_wrong_binding
expect "harmless nested heredoc template in default_labels" 0 provider_nested_heredoc_template_control
expect "budget alert changes resource bound" 1 budget_changes_resource_bound
expect "budget comparison plan deleted" 1 budget_run_deleted
expect "no tofu" 11 none "$T/absent"
expect "restored control" 0 none
exit "$failures"
