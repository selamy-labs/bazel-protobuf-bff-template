mock_provider "google" {}

variables {
  project_id              = "example-bff-hosted-000000"
  region                  = "example-region1"
  registry_id             = "bff-images"
  service_name            = "bff"
  runtime_service_account = join("@", ["bff-runtime", join(".", ["example-bff-hosted-000000", "iam", "gserviceaccount", "com"])])
  notes_bucket            = "example-bff-hosted-000000-notes"
  state_bucket            = "example-bff-hosted-000000-tfstate"
  state_prefix            = "bff-template/prod/service"
  release_topic           = "projects/example-bff-hosted-000000/topics/bff-release"
  deployment_tools_image  = "example-region1-docker.example.invalid/example-bff-hosted-000000/tools/deploy@sha256:0000000000000000000000000000000000000000000000000000000000000000"
  bundle_bucket           = "example-bff-hosted-000000-bundles"
  bundle_object           = "releases/00000000/bundle.zip"
  bundle_generation       = "1"
  bundle_sha256           = "0000000000000000000000000000000000000000000000000000000000000000"
  plan_sha256             = "1111111111111111111111111111111111111111111111111111111111111111"
  image                   = "example-region1-docker.pkg.dev/example-bff-hosted-000000/bff-images/bff@sha256:2222222222222222222222222222222222222222222222222222222222222222"
  expected_changes = {
    "google_cloud_run_v2_service.bff" = ["update"]
  }
  production_approvers = ["user:release-approver@example.invalid"]
}

run "production_trigger_declares_narrow_required_capabilities_and_forbids_self_trigger" {
  command = plan

  assert {
    condition     = google_cloudbuild_trigger.production.approval_config[0].approval_required == true
    error_message = "Production deployments must require native Cloud Build human approval."
  }

  assert {
    condition     = google_cloudbuild_trigger.production.disabled == true
    error_message = "The production trigger must default to disabled until its UUID is recorded."
  }

  assert {
    condition     = google_service_account.production_deployer.account_id == "bff-prod-deployer"
    error_message = "Production deployer must use its own separated identity."
  }

  assert {
    condition     = length(google_project_iam_custom_role.build_read.permissions) == 1 && contains(google_project_iam_custom_role.build_read.permissions, "cloudbuild.builds.get") && !contains(google_project_iam_custom_role.build_read.permissions, "cloudbuild.builds.create")
    error_message = "Production deployer custom build role must permit only cloudbuild.builds.get (no cloudbuild.builds.create)."
  }

  assert {
    condition     = google_storage_bucket_iam_member.bundle_read.condition[0].expression == "resource.name == 'projects/_/buckets/example-bff-hosted-000000-bundles/objects/releases/00000000/bundle.zip'"
    error_message = "Production deployer bundle read must be restricted to the exact approved bundle object."
  }

  assert {
    condition     = google_project_iam_member.production_run.role == "roles/run.developer" && google_project_iam_member.production_run.condition[0].expression == "resource.name == 'projects/example-bff-hosted-000000/locations/example-region1/services/bff'"
    error_message = "Production deployer Cloud Run grant must be CEL-scoped strictly to the target service."
  }

  assert {
    condition     = google_service_account_iam_member.production_runtime_user.role == "roles/iam.serviceAccountUser" && google_service_account_iam_member.production_runtime_user.service_account_id == join("", ["projects/example-bff-hosted-000000/serviceAccounts/", join("@", ["bff-runtime", join(".", ["example-bff-hosted-000000", "iam", "gserviceaccount", "com"])])])
    error_message = "Production deployer actAs grant must bind directly on the declared runtime service account policy."
  }

  assert {
    condition     = length(google_project_iam_custom_role.state_list.permissions) == 1 && contains(google_project_iam_custom_role.state_list.permissions, "storage.objects.list") && !contains(google_project_iam_custom_role.state_list.permissions, "storage.objects.get") && !contains(google_project_iam_custom_role.state_list.permissions, "storage.objects.create") && !contains(google_project_iam_custom_role.state_list.permissions, "storage.objects.delete")
    error_message = "Production deployer state bucket listing role must grant only storage.objects.list."
  }

  assert {
    condition     = google_storage_bucket_iam_member.state_list.bucket == "example-bff-hosted-000000-tfstate" && google_storage_bucket_iam_member.state_list.role == google_project_iam_custom_role.state_list.name
    error_message = "Production deployer state bucket listing binding must target the dedicated state bucket with the minimal custom role."
  }

  assert {
    condition     = google_storage_bucket_iam_member.state_objects.role == "roles/storage.objectUser" && google_storage_bucket_iam_member.state_objects.condition[0].expression == "resource.type == 'storage.googleapis.com/Object' && resource.name.startsWith('projects/_/buckets/example-bff-hosted-000000-tfstate/objects/bff-template/prod/service/')"
    error_message = "Production deployer OpenTofu state object access must be CEL-scoped strictly to objects under the dedicated state prefix."
  }
}

run "rejects_state_bucket_reusing_notes_bucket" {
  command = plan

  variables {
    state_bucket = "example-bff-hosted-000000-notes"
  }

  expect_failures = [
    google_cloudbuild_trigger.production,
  ]
}

run "rejects_state_bucket_reusing_bundle_bucket" {
  command = plan

  variables {
    state_bucket = "example-bff-hosted-000000-bundles"
  }

  expect_failures = [
    google_cloudbuild_trigger.production,
  ]
}

run "rejects_runtime_sa_reusing_production_deployer" {
  command = plan

  variables {
    runtime_service_account = join("@", ["bff-prod-deployer", join(".", ["example-bff-hosted-000000", "iam", "gserviceaccount", "com"])])
  }

  expect_failures = [
    google_service_account_iam_member.production_runtime_user,
  ]
}
