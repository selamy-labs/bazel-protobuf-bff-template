mock_provider "google" {}

variables {
  project_id          = "example-bff-hosted-000000"
  project_name        = "BFF Hosted Example"
  organization_id     = "000000000000"
  billing_account_id  = "000000-000000-000000"
  region              = "example-region1"
  registry_id         = "bff-images"
  notes_bucket_name   = "example-bff-hosted-000000-notes"
  state_bucket_name   = "example-bff-hosted-000000-state"
  handoff_bucket_name = "example-bff-hosted-000000-handoffs"
}

run "foundation_enforces_isolation_private_storage_and_separated_identities" {
  command = plan

  assert {
    condition     = output.dev_service_name == "bff" && google_project_iam_member.dev_deployer_run.condition[0].expression == "resource.name == 'projects/example-bff-hosted-000000/locations/example-region1/services/bff'"
    error_message = "Default foundation dev_service_name and dev_deployer_run CEL condition must compose with deploy/gcp/service name 'bff'."
  }

  assert {
    condition     = google_project.bff.auto_create_network == false
    error_message = "The dedicated project must disable default VPC creation."
  }

  assert {
    condition     = google_artifact_registry_repository.images.format == "DOCKER"
    error_message = "Artifact Registry must be a regional Docker repository."
  }

  assert {
    condition     = google_storage_bucket.notes.uniform_bucket_level_access == true && google_storage_bucket.notes.public_access_prevention == "enforced"
    error_message = "The notes bucket must enforce uniform bucket-level access and block public access."
  }

  assert {
    condition     = google_storage_bucket.release_handoffs.uniform_bucket_level_access == true && google_storage_bucket.release_handoffs.public_access_prevention == "enforced" && google_storage_bucket.release_handoffs.versioning[0].enabled == true && output.handoff_bucket == "example-bff-hosted-000000-handoffs"
    error_message = "The release handoff bucket must enforce UBLA, public access prevention, and object versioning."
  }

  assert {
    condition     = google_storage_bucket_iam_member.release_publisher_handoff_read.bucket == "example-bff-hosted-000000-handoffs" && google_storage_bucket_iam_member.release_publisher_handoff_read.role == "roles/storage.objectViewer"
    error_message = "Release publisher must receive read-only objectViewer on the dedicated release handoffs bucket."
  }

  assert {
    condition     = google_service_account.release_publisher.account_id == "bff-release-publisher" && google_service_account.runtime.account_id == "bff-runtime" && google_service_account.dev_deployer.account_id == "bff-dev-deployer"
    error_message = "Publisher, deployer, and runtime service accounts must stay separated."
  }

  assert {
    condition     = google_service_account_iam_member.dev_deployer_runtime_user.role == "roles/iam.serviceAccountUser" && google_service_account_iam_member.dev_deployer_runtime_user.service_account_id == join("", ["projects/example-bff-hosted-000000/serviceAccounts/", join("@", ["bff-runtime", join(".", ["example-bff-hosted-000000", "iam", "gserviceaccount", "com"])])])
    error_message = "Dev deployer serviceAccountUser must bind directly on the BFF runtime service account policy."
  }

  assert {
    condition     = output.contract_v1.version == 1 && output.contract_v1.project_id == "example-bff-hosted-000000" && output.contract_v1.region == "example-region1"
    error_message = "The contract_v1 output must bind to the dedicated project and schema version 1."
  }
}

run "rejects_handoff_bucket_reusing_notes_bucket" {
  command = plan

  variables {
    handoff_bucket_name = "example-bff-hosted-000000-notes"
  }

  expect_failures = [
    google_storage_bucket.release_handoffs,
  ]
}

run "rejects_handoff_bucket_reusing_state_bucket" {
  command = plan

  variables {
    handoff_bucket_name = "example-bff-hosted-000000-state"
  }

  expect_failures = [
    google_storage_bucket.release_handoffs,
  ]
}
