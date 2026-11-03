#!/usr/bin/env bash
# The hosted example's service module as a reviewed plan (spec 012 FR-012): the inputs validated first, then
# format, validate, the declared resource graph and the mock tests, with no reachable network and no credentials.
# The provider comes from a mirror assembled from the pinned zip and the lock's hash, so nothing is fetched.
# Nothing here applies anything.
#
#   offline.sh <tofu> <provider zip> <service dir> <inputs file> <validator> <rollback inputs> <cancel inputs> <pipeline> <yq>
#
# Exit 0 passed, 1 a finding, 11 nothing could be judged.
set -euo pipefail

tofu="$1"; zip="$2"; service="$3"; inputs="$4"; validator="$5"; rollback="$6"; cancel="$7"; pipeline="$8"; yq="$9"

refuse() { echo "offline: $1" >&2; exit 11; }
[[ -x "$tofu" ]] || refuse "no tofu at $tofu"
[[ -f "$zip" ]] || refuse "no provider zip at $zip"
[[ -x "$validator" ]] || refuse "no inputs validator at $validator"
[[ -f "$rollback" && -f "$cancel" && -f "$pipeline" ]] || refuse "the rollback inputs, the cancel inputs or the pipeline is missing"
[[ -x "$yq" ]] || refuse "no yq at $yq"

work="$(mktemp -d)"
trap 'chmod -R u+w "$work" 2>/dev/null || true; rm -rf "$work"' EXIT

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
# The rollback, cancel, zero-sample, and bounds inputs pass the same funnel before the mock tests read them (FR-009, FR-010).
mkdir -p "$work/deploy/gcp/inputs/testdata"
"$validator" --effective "$work/deploy/gcp/inputs/testdata/rollback.inputs.json" "$rollback" >/dev/null || { echo "offline: the rollback inputs are refused"; exit 1; }
"$validator" --effective "$work/deploy/gcp/inputs/testdata/cancel.inputs.json" "$cancel" >"$work/cancel.out" || { echo "offline: the cancel inputs are refused"; exit 1; }
"$validator" --effective "$work/deploy/gcp/inputs/testdata/zero-sample.inputs.json" "$(dirname "$rollback")/zero-sample.inputs.json" >/dev/null || { echo "offline: the zero sample inputs are refused"; exit 1; }
bounds_src="$(dirname "$rollback")/bounds.inputs.json"
if [[ -f "$bounds_src" ]]; then
  "$validator" --effective "$work/deploy/gcp/inputs/testdata/bounds.inputs.json" "$bounds_src" >/dev/null || { echo "offline: the bounds inputs are refused"; exit 1; }
fi
budget_src="$(dirname "$rollback")/budget.inputs.json"
if [[ -f "$budget_src" ]]; then
  "$validator" --effective "$work/deploy/gcp/inputs/testdata/budget.inputs.json" "$budget_src" >/dev/null || { echo "offline: the budget inputs are refused"; exit 1; }
fi
grep -q '^release.cancel_in_flight=true # from ' "$work/cancel.out" || { echo "offline: the funnel did not carry the cancellation"; exit 1; }

# The pipeline honors the cancellation and sets pipefail across all steps, verifying the saved plan digest before apply.
expected_steps='["validate-inputs","cancel-in-flight","promote-by-digest","plan","apply-behind-the-gate","record"]'
actual_steps="$("$yq" -o=json -I=0 '[.steps[].id]' "$pipeline" 2>/dev/null | tr -d ' \n\t' || true)"
[[ "$actual_steps" == "$expected_steps" ]] || { echo "offline: the pipeline steps are not the enumerated order" >&2; exit 1; }

step_active_script() {
  local step_id="$1"
  "$yq" ".steps[] | select(.id == \"$step_id\") | .args[-1]" "$pipeline" 2>/dev/null | sed -e 's/^[[:space:]]*//' | grep -v '^[[:space:]]*#' | grep -v '^[[:space:]]*$' || true
}

step_raw_script() {
  local step_id="$1"
  "$yq" ".steps[] | select(.id == \"$step_id\") | .args[-1]" "$pipeline" 2>/dev/null || true
}

step_flags() {
  local step_id="$1"
  "$yq" ".steps[] | select(.id == \"$step_id\") | .args[0]" "$pipeline" 2>/dev/null || true
}

for step in validate-inputs cancel-in-flight promote-by-digest plan apply-behind-the-gate record; do
  flags="$(step_flags "$step")"
  [[ "$flags" == *e* ]] || { echo "offline: pipeline step $step does not enable -e in args" >&2; exit 1; }
  [[ "$flags" == *u* ]] || { echo "offline: pipeline step $step does not enable -u in args" >&2; exit 1; }
  script="$(step_active_script "$step")"
  echo "$script" | grep -E -q '^[[:space:]]*set -o pipefail(\b|[[:space:]]|$)' || { echo "offline: pipeline step $step does not set pipefail" >&2; exit 1; }
done

for step in promote-by-digest plan apply-behind-the-gate; do
  script="$(step_active_script "$step")"
  echo "$script" | grep -E -q '^[[:space:]]*\[\[ ! -e /workspace/cancelled \]\] \|\|' || { echo "offline: the step $step ignores a cancellation" >&2; exit 1; }
done

cancel_script="$(step_active_script "cancel-in-flight")"
echo "$cancel_script" | grep -q "release.cancel_in_flight=true" || { echo "offline: the pipeline does not read the cancellation from the funnel" >&2; exit 1; }
# shellcheck disable=SC2016
echo "$cancel_script" | grep -F -q 'id!=${BUILD_ID}' || {
  echo "offline: cancel-in-flight must exclude current build id from cancellation filter" >&2
  exit 1
}

promote_script="$(step_active_script "promote-by-digest")"
echo "$promote_script" | grep -F -q 'crane digest' || {
  echo "offline: promote-by-digest must re-check digest with crane digest" >&2
  exit 1
}

apply_script="$(step_active_script "apply-behind-the-gate")"
if echo "$apply_script" | grep -v '^[[:space:]]*#' | grep -E -q '\b(TF|OPENTOFU)_[a-zA-Z0-9_]*\b'; then
  echo "offline: apply step must not set or export TF_* or OPENTOFU_* environment variables" >&2
  exit 1
fi
for env_entry in $("$yq" '.options.env[]' "$pipeline" 2>/dev/null || true); do
  var_name="${env_entry%%=*}"
  var_name="$(echo "$var_name" | tr -d '[:space:]')"
  if echo "$var_name" | grep -E -q -i '^(TF|OPENTOFU)_'; then
    echo "offline: options.env must not set TF_* or OPENTOFU_* environment variables" >&2
    exit 1
  fi
done
for env_entry in $("$yq" '.steps[].env[]' "$pipeline" 2>/dev/null || true); do
  var_name="${env_entry%%=*}"
  var_name="$(echo "$var_name" | tr -d '[:space:]')"
  if echo "$var_name" | grep -E -q -i '^(TF|OPENTOFU)_'; then
    echo "offline: step env must not set TF_* or OPENTOFU_* environment variables" >&2
    exit 1
  fi
done
tofu_lines="$(echo "$apply_script" | grep -v '^[[:space:]]*#' | grep -E '\btofu\b' || true)"
if echo "$tofu_lines" | grep -E -q '\btofu\b.*(\s|^)(destroy|plan|state|import|refresh|taint|untaint|force-unlock|workspace|output)\b'; then
  echo "offline: apply step contains forbidden tofu subcommand" >&2
  exit 1
fi
static_apply_count="$(echo "$tofu_lines" | grep -c -E '\btofu\b.*(\s|^)apply\b' || true)"
if [[ "$static_apply_count" -ne 1 ]]; then
  echo "offline: apply step must contain exactly one tofu apply, found $static_apply_count" >&2
  exit 1
fi
static_init_count="$(echo "$tofu_lines" | grep -c -E '\btofu\b.*(\s|^)init\b' || true)"
if [[ "$static_init_count" -ne 1 ]]; then
  echo "offline: apply step must contain exactly one tofu init, found $static_init_count" >&2
  exit 1
fi
apply_line_no="$(echo "$apply_script" | grep -n 'tofu apply' | cut -d: -f1 | head -1 || true)"
init_line_no="$(echo "$apply_script" | grep -n -E '\btofu\b.*(\s|^)init\b' | cut -d: -f1 | head -1 || true)"
if [[ -n "$init_line_no" && -n "$apply_line_no" && "$apply_line_no" -lt "$init_line_no" ]]; then
  echo "offline: apply step must not invoke tofu apply before tofu init" >&2
  exit 1
fi
init_cmd="$(echo "$apply_script" | grep -v '^[[:space:]]*#' | grep -E '^[[:space:]]*tofu\b' | grep -E '\binit\b' | head -1 | sed -e 's/^[[:space:]]*//')"
if [[ "$init_cmd" != "tofu init -input=false -lockfile=readonly" ]]; then
  echo "offline: apply step must invoke exact command 'tofu init -input=false -lockfile=readonly'" >&2
  exit 1
fi
apply_cmd="$(echo "$apply_script" | grep -v '^[[:space:]]*#' | grep -E '^[[:space:]]*tofu\b' | grep -E '\bapply\b' | head -1 | sed -e 's/^[[:space:]]*//')"
if [[ "$apply_cmd" != "tofu apply -input=false -auto-approve /workspace/plan.tfplan" ]]; then
  echo "offline: apply step must invoke exact command 'tofu apply -input=false -auto-approve /workspace/plan.tfplan'" >&2
  exit 1
fi
digest_line_no="$(echo "$apply_script" | grep -n 'sha256sum' | grep '_APPROVED_PLAN_SHA256' | cut -d: -f1 | tail -1 || true)"
[[ -n "$apply_line_no" && -n "$digest_line_no" && "$digest_line_no" -lt "$apply_line_no" ]] || {
  echo "offline: apply step must verify plan digest before tofu apply" >&2
  exit 1
}
echo "$apply_script" | grep -F -q '^[0-9a-f]{64}$' || {
  echo "offline: apply step must strictly validate _APPROVED_PLAN_SHA256 format" >&2
  exit 1
}
if echo "$apply_script" | grep -v '^[[:space:]]*#' | grep '_APPROVED_PLAN_SHA256' | grep 'sha256sum' | grep -E -q '\|\|[[:space:]]*(true|:)'; then
  echo "offline: apply step approved plan digest check must not be masked with || true" >&2
  exit 1
fi
if echo "$apply_script" | grep -v '^[[:space:]]*#' | grep -E '\bsha256sum\b' | grep -v -q -- '--strict'; then
  echo "offline: apply step must invoke sha256sum with --strict" >&2
  exit 1
fi
# shellcheck disable=SC2016
if ! echo "$apply_script" | grep -v '^[[:space:]]*#' | grep -F -q 'grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256'; then
  echo "offline: apply step must verify plan sidecar checksum against approved digest" >&2
  exit 1
fi

record_script="$(step_active_script "record")"
echo "$record_script" | grep -F -q 'gcloud run services describe bff' || {
  echo "offline: record step must describe service after apply" >&2
  exit 1
}
echo "$record_script" | grep -F -q 'cancelled=true' || {
  echo "offline: record step must record cancellation status" >&2
  exit 1
}

# Executable pipeline witness harness: verify step execution against mock command stubs in an isolated sandbox.
pipe_work="$work/pipe_harness"
parent_state="$pipe_work/parent_state"
mkdir -p "$pipe_work/bin" "$pipe_work/workspace" "$pipe_work/service" "$parent_state"
parent_approval_file="$parent_state/approved_plan.sha256"
stub_log="$pipe_work/stub.log"

cat >"$pipe_work/bin/gcloud" <<'STUB_GCLOUD'
#!/usr/bin/env bash
set -euo pipefail
echo "gcloud $*" >> "${STUB_LOG:-/dev/null}"
if [[ "$*" == *"builds list"* ]]; then
  echo "other-build-id-123"
fi
if [[ "$*" == *"run services describe"* ]]; then
  echo "{}"
fi
if [[ "${1:-}" == "storage" && "${2:-}" == "cp" ]]; then
  src="$3"
  dst="$4"
  if [[ -n "${STORAGE_ROOT:-}" ]]; then
    if [[ "$src" == gs://* ]]; then
      src="$STORAGE_ROOT/${src#gs://}"
    fi
    if [[ "$dst" == gs://* ]]; then
      dst="$STORAGE_ROOT/${dst#gs://}"
    fi
  fi
  mkdir -p "$(dirname "$dst")"
  cp "$src" "$dst"
fi
STUB_GCLOUD
chmod +x "$pipe_work/bin/gcloud"

cat >"$pipe_work/bin/crane" <<'STUB_CRANE'
#!/usr/bin/env bash
set -euo pipefail
echo "crane $*" >> "${STUB_LOG:-/dev/null}"
if [[ "$1" == "digest" ]]; then
  echo "${_SERVING_DIGEST:-sha256:0000000000000000000000000000000000000000000000000000000000000000}"
fi
STUB_CRANE
chmod +x "$pipe_work/bin/crane"

cat >"$pipe_work/bin/tofu" <<'STUB_TOFU'
#!/usr/bin/env bash
set -euo pipefail
echo "tofu $*" >> "${STUB_LOG:-/dev/null}"
step="${CURRENT_STEP_ID:-}"

args=("$@")
subcmd=""
subcmd_idx=-1
global_flags=()
i=0
while [[ $i -lt ${#args[@]} ]]; do
  arg="${args[$i]}"
  if [[ "$arg" == -chdir=* || "$arg" == -no-color || "$arg" == -help || "$arg" == --help || "$arg" == -h || "$arg" == -version || "$arg" == -v || "$arg" == --version ]]; then
    global_flags+=("$arg")
    i=$((i + 1))
  elif [[ "$arg" == "-chdir" ]]; then
    global_flags+=("$arg" "${args[$((i+1))]:-}")
    i=$((i + 2))
  elif [[ "$arg" == -* && -z "$subcmd" ]]; then
    global_flags+=("$arg")
    i=$((i + 1))
  else
    subcmd="$arg"
    subcmd_idx=$i
    break
  fi
done

sub_args=()
if [[ $subcmd_idx -ge 0 ]]; then
  sub_args=("${args[@]:$((subcmd_idx + 1))}")
fi

current_plan_sha=""
if [[ -f "$WORKSPACE_DIR/plan.tfplan" ]]; then
  current_plan_sha="$(sha256sum "$WORKSPACE_DIR/plan.tfplan" | awk '{print $1}')"
fi
echo "tofu_invocation subcmd=$subcmd target_digest=$current_plan_sha" >> "${STUB_LOG:-/dev/null}"

if [[ "$step" == "apply-behind-the-gate" ]]; then
  if env | grep -E -q '^(TF|OPENTOFU)_[a-zA-Z0-9_]+='; then
    echo "mock tofu: TF_* and OPENTOFU_* environment variables are forbidden during apply step" >&2
    exit 1
  fi
  if [[ "$subcmd" != "init" && "$subcmd" != "apply" ]]; then
    echo "mock tofu: subcommand '$subcmd' is forbidden during apply step" >&2
    exit 1
  fi
  for arg in "${args[@]}"; do
    if [[ "$arg" == -out=* ]]; then
      echo "mock tofu: overwriting plan artifact is forbidden during apply step" >&2
      exit 1
    fi
  done
  apply_state_file="${WORKSPACE_DIR:-/tmp}/.tofu_apply_state"
  if [[ "$subcmd" == "init" ]]; then
    if [[ -f "$apply_state_file" ]] && grep -q '^init$' "$apply_state_file"; then
      echo "mock tofu: duplicate tofu init during apply step" >&2
      exit 1
    fi
    if [[ -f "$apply_state_file" ]] && grep -q '^apply$' "$apply_state_file"; then
      echo "mock tofu: tofu init after tofu apply during apply step" >&2
      exit 1
    fi
    if [[ ${#global_flags[@]} -gt 0 ]]; then
      echo "mock tofu: global options forbidden on init during apply step" >&2
      exit 1
    fi
    expected_init_args=("-input=false" "-lockfile=readonly")
    if [[ "${#sub_args[@]}" -ne "${#expected_init_args[@]}" ]]; then
      echo "mock tofu: tofu init must accept only exact arguments: init ${expected_init_args[*]}" >&2
      exit 1
    fi
    for idx in "${!expected_init_args[@]}"; do
      if [[ "${sub_args[$idx]}" != "${expected_init_args[$idx]}" ]]; then
        echo "mock tofu: unexpected init argument at index $idx: expected ${expected_init_args[$idx]}, got ${sub_args[$idx]}" >&2
        exit 1
      fi
    done
    echo "init" >> "$apply_state_file"
  fi
  if [[ "$subcmd" == "apply" ]]; then
    if [[ ! -f "$apply_state_file" ]] || ! grep -q '^init$' "$apply_state_file"; then
      echo "mock tofu: tofu apply before tofu init during apply step" >&2
      exit 1
    fi
    if grep -q '^apply$' "$apply_state_file"; then
      echo "mock tofu: duplicate tofu apply during apply step" >&2
      exit 1
    fi
    if [[ ${#global_flags[@]} -gt 0 ]]; then
      echo "mock tofu: global options forbidden on apply during apply step" >&2
      exit 1
    fi
    expected_args=("-input=false" "-auto-approve" "$WORKSPACE_DIR/plan.tfplan")
    if [[ "${#sub_args[@]}" -ne "${#expected_args[@]}" ]]; then
      echo "mock tofu: tofu apply must accept only exact arguments: apply ${expected_args[*]}" >&2
      exit 1
    fi
    for idx in "${!expected_args[@]}"; do
      actual="${sub_args[$idx]}"
      expected="${expected_args[$idx]}"
      if [[ "$actual" == -* ]]; then
        if [[ "$actual" != "$expected" ]]; then
          echo "mock tofu: unexpected argument at index $idx: expected $expected, got $actual" >&2
          exit 1
        fi
      else
        if [[ "$actual" != "$expected" ]]; then
          echo "mock tofu: unexpected argument path at index $idx: expected $expected, got $actual" >&2
          exit 1
        fi
      fi
    done
    if [[ ! -f "$WORKSPACE_DIR/plan.tfplan" ]]; then
      echo "mock tofu: missing plan artifact at $WORKSPACE_DIR/plan.tfplan" >&2
      exit 1
    fi
    if [[ -f "${PARENT_APPROVAL_FILE:-}" ]]; then
      expected_parent_sha="$(tr -d '[:space:]' < "$PARENT_APPROVAL_FILE")"
      if [[ -n "$expected_parent_sha" && "$current_plan_sha" != "$expected_parent_sha" ]]; then
        echo "mock tofu: plan artifact digest $current_plan_sha does not match parent approved digest $expected_parent_sha" >&2
        exit 1
      fi
    fi
    echo "apply" >> "$apply_state_file"
  fi
fi

for arg in "${args[@]}"; do
  if [[ "$arg" == -out=* ]]; then
    out_file="${arg#-out=}"
    echo "mock-plan-artifact" > "$out_file"
  fi
done
STUB_TOFU
chmod +x "$pipe_work/bin/tofu"

cat >"$pipe_work/bin/bazel" <<'STUB_BAZEL'
#!/usr/bin/env bash
set -euo pipefail
echo "bazel $*" >> "${STUB_LOG:-/dev/null}"
if [[ "${BAZEL_MOCK_FAIL:-0}" == "1" ]]; then
  exit 1
fi
echo 'release.serving_digest="sha256:0000" # from example'
STUB_BAZEL
chmod +x "$pipe_work/bin/bazel"

exec_step() {
  local step_id="$1"
  local approved_sha="${2:-}"
  local bazel_fail="${3:-0}"
  local storage_url="${4:-}"
  local raw_script flags script_adapted rc=0
  raw_script="${5:-$(step_raw_script "$step_id")}"
  flags="$(step_flags "$step_id")"
  # Adapt /workspace and service directory paths to run within unprivileged pipe_work sandbox
  script_adapted="${raw_script//\/workspace/$pipe_work\/workspace}"
  script_adapted="${script_adapted//deploy\/gcp\/service/$pipe_work\/service}"
  (
    unset _APPROVED_PLAN_SHA256 _PLAN_STORAGE_URL 2>/dev/null || true
    if [[ "$step_id" == "apply-behind-the-gate" ]]; then
      rm -f "$pipe_work/workspace/.tofu_apply_state"
      for tf_var in $(env | grep -oE '^(TF|OPENTOFU)_[a-zA-Z0-9_]+' || true); do
        unset "$tf_var" 2>/dev/null || true
      done
    fi
    while IFS= read -r env_entry; do
      [[ -n "$env_entry" ]] || continue
      env_entry="${env_entry//\$\{_APPROVED_PLAN_SHA256\}/$approved_sha}"
      env_entry="${env_entry//\$\{_PLAN_STORAGE_URL\}/$storage_url}"
      export "${env_entry?}"
    done < <("$yq" '.options.env[]' "$pipeline" 2>/dev/null || true)
    while IFS= read -r env_entry; do
      [[ -n "$env_entry" ]] || continue
      env_entry="${env_entry//\$\{_APPROVED_PLAN_SHA256\}/$approved_sha}"
      env_entry="${env_entry//\$\{_PLAN_STORAGE_URL\}/$storage_url}"
      export "${env_entry?}"
    done < <("$yq" ".steps[] | select(.id == \"$step_id\") | .env[]" "$pipeline" 2>/dev/null || true)
    export PATH="$pipe_work/bin:$PATH"
    export CURRENT_STEP_ID="$step_id"
    export WORKSPACE_DIR="$pipe_work/workspace"
    export STUB_LOG="$stub_log"
    export STORAGE_ROOT="$pipe_work/storage"
    export TRIGGER_NAME="test-trigger"
    export BUILD_ID="test-build"
    export _TOOLS_IMAGE="test-image"
    export _SOURCE_IMAGE="test-source"
    export _SERVING_DIGEST="sha256:0000000000000000000000000000000000000000000000000000000000000000"
    export _ARTIFACT_REGISTRY="test-registry"
    export _INPUTS_FILE="$inputs"
    export _REGION="us-central1"
    export _PREVIOUS_DIGEST="sha256:1111111111111111111111111111111111111111111111111111111111111111"
    export COMMIT_SHA="test-commit"
    export PARENT_APPROVAL_FILE="$parent_approval_file"
    export BAZEL_MOCK_FAIL="$bazel_fail"
    bash "$flags" -c "$script_adapted"
  ) || rc=$?
  return "$rc"
}

# 1. Witness that validate-inputs fails when bazel validator pipeline fails (verifying pipefail and -e execution)
exec_step "validate-inputs" "" "1" >/dev/null 2>&1 && v_rc=0 || v_rc=$?
if [[ "$v_rc" -eq 0 ]]; then
  echo "offline: pipeline step validate-inputs does not fail when validator pipeline fails (pipefail or -e inactive)" >&2
  exit 1
fi

# 2. Witness cancel-in-flight step behavior
printf 'release.cancel_in_flight=true # from funnel\n' > "$pipe_work/workspace/validate.out"
rm -f "$pipe_work/workspace/cancelled"
: > "$stub_log"
exec_step "cancel-in-flight" >/dev/null 2>&1 || true
if [[ ! -f "$pipe_work/workspace/cancelled" ]] || ! grep -q "gcloud builds cancel" "$stub_log"; then
  echo "offline: cancel-in-flight step did not execute cancellation" >&2
  exit 1
fi
if ! grep -q "id!=test-build" "$stub_log"; then
  echo "offline: cancel-in-flight did not exclude current build id from cancellation filter" >&2
  exit 1
fi

# 3. Witness cancellation guard on promote-by-digest, plan, and apply-behind-the-gate
for step in promote-by-digest plan apply-behind-the-gate; do
  touch "$pipe_work/workspace/cancelled"
  : > "$stub_log"
  exec_step "$step" >/dev/null 2>&1 || { echo "offline: the step $step ignores a cancellation" >&2; exit 1; }
  if grep -q -E '(crane|tofu)' "$stub_log"; then
    echo "offline: the step $step ignores a cancellation" >&2
    exit 1
  fi
done
rm -f "$pipe_work/workspace/cancelled"

# 3a. Witness promote-by-digest in normal mode (crane copy and digest verification)
: > "$stub_log"
exec_step "promote-by-digest" >/dev/null 2>&1 || {
  echo "offline: promote-by-digest failed in normal mode" >&2
  exit 1
}
if ! grep -q "crane copy" "$stub_log"; then
  echo "offline: promote-by-digest did not execute crane copy" >&2
  exit 1
fi
if ! grep -q "crane digest" "$stub_log"; then
  echo "offline: promote-by-digest did not execute crane digest re-check" >&2
  exit 1
fi

# 4. Witness plan-mode plan generation & storage upload roundtrip
storage_dir="$pipe_work/storage"
mkdir -p "$storage_dir"
rm -f "$pipe_work/workspace/plan.tfplan" "$pipe_work/workspace/plan.tfplan.sha256"
: > "$stub_log"
exec_step "plan" "" "0" "gs://mock-bucket/release-1" >/dev/null 2>&1 || {
  echo "offline: plan step failed in plan mode" >&2
  exit 1
}
if [[ ! -f "$storage_dir/mock-bucket/release-1/plan.tfplan" ]] || [[ ! -f "$storage_dir/mock-bucket/release-1/plan.tfplan.sha256" ]]; then
  echo "offline: plan step did not upload plan and sha256 sidecar to storage" >&2
  exit 1
fi
if ! grep -q "tofu plan" "$stub_log"; then
  echo "offline: plan step did not execute tofu plan in plan mode" >&2
  exit 1
fi

generated_sha="$(sha256sum "$storage_dir/mock-bucket/release-1/plan.tfplan" | awk '{print $1}')"

# 4a. Witness apply-behind-the-gate skips when _APPROVED_PLAN_SHA256 is empty (plan mode)
: > "$stub_log"
exec_step "apply-behind-the-gate" "" >/dev/null 2>&1 || {
  echo "offline: apply step failed on empty approval" >&2
  exit 1
}
if grep -q "tofu apply" "$stub_log"; then
  echo "offline: apply step ran tofu apply without approved plan digest" >&2
  exit 1
fi

# 4b. Witness apply mode in fresh workspace: download plan & sidecar from storage, no re-planning
rm -f "$pipe_work/workspace/plan.tfplan" "$pipe_work/workspace/plan.tfplan.sha256"
: > "$stub_log"
exec_step "plan" "$generated_sha" "0" "gs://mock-bucket/release-1" >/dev/null 2>&1 || {
  echo "offline: plan step in apply mode failed to download plan from storage" >&2
  exit 1
}
if grep -q "tofu plan" "$stub_log"; then
  echo "offline: plan step executed tofu plan during apply mode instead of using stored plan" >&2
  exit 1
fi
if [[ ! -f "$pipe_work/workspace/plan.tfplan" ]] || [[ ! -f "$pipe_work/workspace/plan.tfplan.sha256" ]]; then
  echo "offline: plan step in apply mode did not download /workspace/plan.tfplan and sidecar" >&2
  exit 1
fi

# 4c. Witness apply-behind-the-gate verification and apply invocation with exact flags
chmod 755 "$parent_state" 2>/dev/null || true
echo "$generated_sha" > "$parent_approval_file"
chmod 444 "$parent_approval_file"
chmod 555 "$parent_state"

: > "$stub_log"
exec_step "apply-behind-the-gate" "$generated_sha" >/dev/null 2>&1 || {
  echo "offline: apply step failed to apply approved plan artifact" >&2
  exit 1
}
chmod 755 "$parent_state" 2>/dev/null || true
if grep -q "tofu plan" "$stub_log" || grep -q '^tofu_invocation subcmd=plan ' "$stub_log"; then
  echo "offline: apply step executed forbidden tofu plan" >&2
  exit 1
fi
if grep -E -q '^tofu_invocation subcmd=(destroy|plan|state|import|refresh|taint|untaint|force-unlock|workspace|output) ' "$stub_log"; then
  echo "offline: apply step executed forbidden tofu subcommand" >&2
  exit 1
fi
apply_count="$(grep -c '^tofu_invocation subcmd=apply ' "$stub_log" || true)"
if [[ "$apply_count" -ne 1 ]]; then
  echo "offline: apply step must execute exactly one tofu apply, found $apply_count" >&2
  exit 1
fi
init_count="$(grep -c '^tofu_invocation subcmd=init ' "$stub_log" || true)"
if [[ "$init_count" -gt 1 ]]; then
  echo "offline: apply step must execute at most one tofu init, found $init_count" >&2
  exit 1
fi
init_idx="$(grep -n '^tofu_invocation subcmd=init ' "$stub_log" | cut -d: -f1 | head -1 || true)"
apply_idx="$(grep -n '^tofu_invocation subcmd=apply ' "$stub_log" | cut -d: -f1 | head -1 || true)"
if [[ -n "$init_idx" && -n "$apply_idx" && "$apply_idx" -lt "$init_idx" ]]; then
  echo "offline: apply step executed tofu apply before tofu init" >&2
  exit 1
fi
if ! grep -q "^tofu init -input=false -lockfile=readonly$" "$stub_log"; then
  echo "offline: apply step did not invoke tofu init with exact arguments -input=false -lockfile=readonly" >&2
  exit 1
fi
if ! grep -q "^tofu apply -input=false -auto-approve $pipe_work/workspace/plan.tfplan$" "$stub_log"; then
  echo "offline: apply step did not invoke tofu apply with required -input=false -auto-approve flags and approved plan" >&2
  exit 1
fi
if ! grep -q "^tofu_invocation subcmd=apply target_digest=$generated_sha$" "$stub_log"; then
  echo "offline: tofu apply was not invoked with approved plan artifact digest $generated_sha" >&2
  exit 1
fi
current_applied_sha="$(sha256sum "$pipe_work/workspace/plan.tfplan" | awk '{print $1}')"
if [[ "$current_applied_sha" != "$generated_sha" ]]; then
  echo "offline: plan artifact was modified before apply" >&2
  exit 1
fi

# 4d. Witness malformed approval formats rejected (newlines, comments, spaces, non-hex, short/long)
null_sha="$(sha256sum /dev/null | awk '{print $1}')"
for malformed in \
  "${null_sha}  /dev/null"$'\n'"#" \
  " $generated_sha" \
  "${generated_sha} " \
  "0000"; do
  : > "$stub_log"
  exec_step "apply-behind-the-gate" "$malformed" >/dev/null 2>&1 && mal_rc=0 || mal_rc=$?
  if [[ "$mal_rc" -eq 0 ]] || grep -q "tofu apply" "$stub_log"; then
    echo "offline: apply step permitted malformed approval value: $malformed" >&2
    exit 1
  fi
done

# 4e. Witness wrong digest rejected
: > "$stub_log"
exec_step "apply-behind-the-gate" "0000000000000000000000000000000000000000000000000000000000000000" >/dev/null 2>&1 && wrong_rc=0 || wrong_rc=$?
if [[ "$wrong_rc" -eq 0 ]] || grep -q "tofu apply" "$stub_log"; then
  echo "offline: apply step permitted apply with wrong plan digest" >&2
  exit 1
fi

# 4f. Witness record step in normal mode (service describe and deployment record)
: > "$stub_log"
exec_step "record" "$generated_sha" >/dev/null 2>&1 || {
  echo "offline: record step failed in normal mode" >&2
  exit 1
}
if ! grep -q "gcloud run services describe bff" "$stub_log"; then
  echo "offline: record step did not describe service after apply" >&2
  exit 1
fi
if [[ ! -f "$pipe_work/workspace/service.after.json" ]] || [[ ! -f "$pipe_work/workspace/deployment.json" ]]; then
  echo "offline: record step did not write service.after.json and deployment.json" >&2
  exit 1
fi
if ! grep -q '"cancelled":false' "$pipe_work/workspace/deployment.json"; then
  echo "offline: record step did not record cancelled=false in normal mode" >&2
  exit 1
fi

# 4g. Witness record step in cancelled mode (no service describe, records cancelled=true)
touch "$pipe_work/workspace/cancelled"
: > "$stub_log"
exec_step "record" "$generated_sha" >/dev/null 2>&1 || {
  echo "offline: record step failed in cancelled mode" >&2
  exit 1
}
if grep -q "gcloud run services describe" "$stub_log"; then
  echo "offline: record step must not describe service when cancelled" >&2
  exit 1
fi
if ! grep -q '"cancelled":true' "$pipe_work/workspace/deployment.json"; then
  echo "offline: record step did not record cancelled=true in cancelled mode" >&2
  exit 1
fi
rm -f "$pipe_work/workspace/cancelled"

# 4h. Direct positive and negative stub tests for flag parsing and step guards (BFF-TOFU-R7-SOL61-02)
: > "$stub_log"
CURRENT_STEP_ID="plan" WORKSPACE_DIR="$pipe_work/workspace" STUB_LOG="$stub_log" \
  "$pipe_work/bin/tofu" -chdir=. init -backend=false >/dev/null 2>&1 || {
  echo "offline: STUB_TOFU failed on leading -chdir=. global option in plan step" >&2
  exit 1
}
if ! grep -q "^tofu_invocation subcmd=init " "$stub_log"; then
  echo "offline: STUB_TOFU did not record structured invocation for -chdir=. init" >&2
  exit 1
fi

: > "$stub_log"
CURRENT_STEP_ID="apply-behind-the-gate" WORKSPACE_DIR="$pipe_work/workspace" STUB_LOG="$stub_log" \
  "$pipe_work/bin/tofu" -chdir=. plan >/dev/null 2>&1 && stub_neg_rc=0 || stub_neg_rc=$?
if [[ "$stub_neg_rc" -eq 0 ]]; then
  echo "offline: STUB_TOFU accepted forbidden -chdir=. plan in apply-behind-the-gate" >&2
  exit 1
fi

# 4i. Witness that state-mutating commands, duplicate apply, and TF_CLI_ARGS in apply step fail (BFF-TOFU-R7-001)
exec_mutated_apply() {
  local extra_line="$1"
  local raw_script repl
  raw_script="$(step_raw_script "apply-behind-the-gate")"
  repl="$(printf '%s\ntofu apply -input=false -auto-approve /workspace/plan.tfplan' "$extra_line")"
  raw_script="${raw_script//tofu apply -input=false -auto-approve \/workspace\/plan.tfplan/$repl}"
  exec_step "apply-behind-the-gate" "$generated_sha" "0" "" "$raw_script"
}

for mut_cmd in \
  "tofu destroy -auto-approve -input=false" \
  "tofu state rm google_cloud_run_v2_service.bff" \
  "tofu import google_cloud_run_v2_service.bff x" \
  "tofu apply -input=false -auto-approve /workspace/plan.tfplan" \
  "export TF_CLI_ARGS_apply=-lock=false" \
  "export TF_WORKSPACE=default" \
  "export TF_CLI_CONFIG_FILE=/tmp/override.tfrc" \
  "export TF_DATA_DIR=/tmp/data" \
  "export OPENTOFU_LOG=DEBUG" \
  "tofu init -migrate-state" \
  "tofu init -backend-config=foo.tfbackend" \
  "tofu init -reconfigure" \
  "tofu init -upgrade"; do
  : > "$stub_log"
  exec_mutated_apply "$mut_cmd" >/dev/null 2>&1 && mut_rc=0 || mut_rc=$?
  if [[ "$mut_rc" -eq 0 ]]; then
    echo "offline: apply step permitted forbidden command/env mutation: $mut_cmd" >&2
    exit 1
  fi
done

config="$work/deploy/gcp/service"
export HOME="$work/home" TF_PLUGIN_CACHE_DIR="" TF_CLI_CONFIG_FILE="$work/cli.tfrc" HTTPS_PROXY=http://127.0.0.1:9 HTTP_PROXY=http://127.0.0.1:9 NO_PROXY="" GOOGLE_APPLICATION_CREDENTIALS="" GOOGLE_CREDENTIALS="" CLOUDSDK_CONFIG="$work/home" TF_INPUT=0
mkdir -p "$HOME"
rm -rf "$config/.terraform"

# Hermetic service and pipeline configuration verification via pinned validator Go tool
"$validator" --check-pipeline "$pipeline" || exit 1
"$validator" --check-service "$config" || exit 1

"$tofu" -chdir="$config" fmt -check -recursive >/dev/null || { echo "offline: the configuration is not formatted"; exit 1; }
"$tofu" -chdir="$config" init -input=false -lockfile=readonly >/dev/null || refuse "init did not complete from the mirror"
"$tofu" -chdir="$config" validate >/dev/null || { echo "offline: the configuration does not validate"; exit 1; }

# Native tofu test with JSON stream output, replacing text scanning with structured verification via pinned validator
"$tofu" -chdir="$config" test -json -verbose -var "inputs_file=../inputs/example.inputs.json" >"$work/test.jsonl" 2>"$work/test.err" || {
  echo "offline: a mock test failed"
  exit 1
}

"$validator" --check-test-stream "$work/test.jsonl" || exit 1

# Positive control: verify that refactoring versions.tf to route through local.inputs.contract passes.
refactor_work="$work/refactor_control"
mkdir -p "$refactor_work"
cp -rL "$config/." "$refactor_work/"
sed -i \
  -e 's/jsondecode(file(var.inputs_file))\.contract\.project_id/local.inputs.contract.project_id/g' \
  -e 's/jsondecode(file(var.inputs_file))\.contract\.region/local.inputs.contract.region/g' \
  "$refactor_work/versions.tf"
"$validator" --check-service "$refactor_work" || {
  echo "offline: equivalent local.inputs.contract refactor failed service check" >&2
  exit 1
}
"$tofu" -chdir="$refactor_work" validate >/dev/null || {
  echo "offline: equivalent local.inputs.contract refactor failed tofu validate" >&2
  exit 1
}

echo "offline: the service module validates and its mock tests pass offline against the synthetic inputs; nothing applied"
