# Native human-approved Cloud Build trigger and isolated production deployer identity.
# Explicitly excludes cloudbuild.builds.create so no build identity can self-trigger or approve.
resource "google_service_account" "production_deployer" {
  project      = var.project_id
  account_id   = "bff-prod-deployer"
  display_name = "BFF production deployment (Cloud Build)"
}

resource "google_project_iam_member" "production_logs" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.production_deployer.email}"
}

resource "google_project_iam_custom_role" "build_read" {
  project     = var.project_id
  role_id     = "bffProductionBuildRead"
  title       = "Read BFF Cloud Build approval evidence"
  permissions = ["cloudbuild.builds.get"]
}

resource "google_project_iam_member" "build_read" {
  project = var.project_id
  role    = google_project_iam_custom_role.build_read.name
  member  = "serviceAccount:${google_service_account.production_deployer.email}"
}

resource "google_storage_bucket_iam_member" "bundle_read" {
  bucket = var.bundle_bucket
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.production_deployer.email}"

  condition {
    title       = "bff_exact_release_bundle"
    description = "Read the reviewed release bundle object only."
    expression  = "resource.name == 'projects/_/buckets/${var.bundle_bucket}/objects/${var.bundle_object}'"
  }
}

resource "google_artifact_registry_repository_iam_member" "image_read" {
  project    = var.project_id
  location   = var.region
  repository = var.registry_id
  role       = "roles/artifactregistry.reader"
  member     = "serviceAccount:${google_service_account.production_deployer.email}"
}

resource "google_project_iam_member" "production_run" {
  project = var.project_id
  role    = "roles/run.developer"
  member  = "serviceAccount:${google_service_account.production_deployer.email}"

  condition {
    title       = "bff_prod_service_only"
    description = "Restrict production deployment mutations to the single declared BFF Cloud Run service."
    expression  = "resource.name == 'projects/${var.project_id}/locations/${var.region}/services/${var.service_name}'"
  }
}

resource "google_service_account_iam_member" "production_runtime_user" {
  service_account_id = "projects/${var.project_id}/serviceAccounts/${var.runtime_service_account}"
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.production_deployer.email}"

  lifecycle {
    precondition {
      condition     = !startswith(var.runtime_service_account, "${google_service_account.production_deployer.account_id}@")
      error_message = "Production runtime_service_account must stay separate from bff-prod-deployer."
    }
  }
}

# OpenTofu 1.13.2 GCS backend Workspaces lists the state prefix during init even for the default
# workspace; storage.objects.list is evaluated on the bucket resource and cannot match an object
# prefix condition. Grant ONLY storage.objects.list on the dedicated state bucket (metadata listing
# only, zero object content read/write/delete outside state_prefix) while keeping notes and bundle
# buckets completely separate.
resource "google_project_iam_custom_role" "state_list" {
  project     = var.project_id
  role_id     = "bffProductionStateList"
  title       = "List BFF OpenTofu state bucket objects"
  permissions = ["storage.objects.list"]
}

resource "google_storage_bucket_iam_member" "state_list" {
  bucket = var.state_bucket
  role   = google_project_iam_custom_role.state_list.name
  member = "serviceAccount:${google_service_account.production_deployer.email}"
}

resource "google_storage_bucket_iam_member" "state_objects" {
  bucket = var.state_bucket
  role   = "roles/storage.objectUser"
  member = "serviceAccount:${google_service_account.production_deployer.email}"

  condition {
    title       = "bff_prod_service_state_prefix_only"
    description = "Restrict OpenTofu remote state object read/write/delete strictly to the dedicated production service state prefix."
    expression  = "resource.type == 'storage.googleapis.com/Object' && resource.name.startsWith('projects/_/buckets/${var.state_bucket}/objects/${var.state_prefix}/')"
  }
}

resource "google_project_iam_member" "production_approver" {
  for_each = var.production_approvers
  project  = var.project_id
  role     = "roles/cloudbuild.builds.approver"
  member   = each.value
}

resource "google_cloudbuild_trigger" "production" {
  project         = var.project_id
  location        = var.region
  name            = "bff-production-release"
  description     = "Native human-approved exact saved-plan BFF deployment"
  service_account = google_service_account.production_deployer.id
  disabled        = !var.enable_trigger

  lifecycle {
    precondition {
      condition     = startswith(var.release_topic, "projects/${var.project_id}/topics/")
      error_message = "The release topic must belong to the deployment project."
    }
    precondition {
      condition     = startswith(var.image, "${var.region}-docker.pkg.dev/${var.project_id}/${var.registry_id}/")
      error_message = "The runtime image must belong to this project's private BFF registry."
    }
    precondition {
      condition     = !var.enable_trigger || can(regex("^[0-9a-f-]{36}$", var.expected_trigger_id))
      error_message = "expected_trigger_id must be a recorded Cloud Build UUID before enabling the trigger."
    }
    precondition {
      condition     = var.state_bucket != var.notes_bucket
      error_message = "Production OpenTofu state_bucket must be separate from runtime notes_bucket."
    }
    precondition {
      condition     = var.state_bucket != var.bundle_bucket
      error_message = "Writable OpenTofu state_bucket must be separate from immutable reviewed bundle_bucket."
    }
  }

  pubsub_config {
    topic = var.release_topic
  }

  approval_config {
    approval_required = true
  }

  build {
    options {
      logging = "CLOUD_LOGGING_ONLY"
    }

    step {
      name       = var.deployment_tools_image
      entrypoint = "python3"
      args       = ["-c", file("${path.module}/execute.py")]
      env = [
        "BUILD_ID=$BUILD_ID",
        "BFF_PROJECT=${var.project_id}",
        "BFF_REGION=${var.region}",
        "BFF_TRIGGER_ID=${var.expected_trigger_id}",
        "BFF_DEPLOYER=${google_service_account.production_deployer.id}",
        "BFF_APPROVERS=${jsonencode([for p in sort(tolist(var.production_approvers)) : trimprefix(p, "user:")])}",
        "BFF_BUNDLE_URI=gs://${var.bundle_bucket}/${var.bundle_object}#${var.bundle_generation}",
        "BFF_BUNDLE_SHA256=${var.bundle_sha256}",
        "BFF_PLAN_SHA256=${var.plan_sha256}",
        "BFF_IMAGE=${var.image}",
        "BFF_SERVICE=${var.service_name}",
        "BFF_RUNTIME_SA=${var.runtime_service_account}",
        "BFF_NOTES_BUCKET=${var.notes_bucket}",
        "BFF_SCOPE=${jsonencode(var.expected_changes)}",
        "BFF_GUARD_B64=${base64encode(file("${path.module}/plan_guard.py"))}",
      ]
    }
  }
}
