# Dedicated non-shared foundation for the BFF template hosted rollout.
# Publisher, deployer, and runtime service accounts hold strictly disjoint permissions.
resource "google_project" "bff" {
  project_id          = var.project_id
  name                = var.project_name
  org_id              = var.organization_id
  billing_account     = var.billing_account_id
  auto_create_network = false
  lifecycle {
    prevent_destroy = true
  }
}

resource "google_project_service" "required" {
  for_each = toset([
    "artifactregistry.googleapis.com",
    "cloudbuild.googleapis.com",
    "compute.googleapis.com",
    "iam.googleapis.com",
    "run.googleapis.com",
    "storage.googleapis.com",
  ])
  project            = google_project.bff.project_id
  service            = each.value
  disable_on_destroy = false
}

resource "google_artifact_registry_repository" "images" {
  project       = google_project.bff.project_id
  location      = var.region
  repository_id = var.registry_id
  format        = "DOCKER"
  description   = "Private BFF template application images"
  depends_on    = [google_project_service.required["artifactregistry.googleapis.com"]]
}

resource "google_storage_bucket" "notes" {
  name                        = var.notes_bucket_name
  project                     = google_project.bff.project_id
  location                    = var.region
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"
  force_destroy               = false

  versioning {
    enabled = true
  }

  depends_on = [google_project_service.required["storage.googleapis.com"]]
}

resource "google_storage_bucket" "release_handoffs" {
  name                        = var.handoff_bucket_name
  project                     = google_project.bff.project_id
  location                    = var.region
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"
  force_destroy               = false

  versioning {
    enabled = true
  }

  lifecycle {
    precondition {
      condition     = var.handoff_bucket_name != var.notes_bucket_name
      error_message = "Release handoff bucket must be separate from the runtime notes bucket."
    }
    precondition {
      condition     = var.handoff_bucket_name != var.state_bucket_name
      error_message = "Release handoff bucket must be separate from the OpenTofu state bucket."
    }
  }

  depends_on = [google_project_service.required["storage.googleapis.com"]]
}

# Image publisher: reads immutable handoff archives from release_handoffs, pushes verified OCI
# digests to the one image repository, and writes Cloud Build logs. Holds no deploy, runtime,
# notes-bucket, handoff-write, or build-approval rights.
resource "google_service_account" "release_publisher" {
  project      = google_project.bff.project_id
  account_id   = "bff-release-publisher"
  display_name = "BFF image publisher (Cloud Build)"
  depends_on   = [google_project_service.required["iam.googleapis.com"]]
}

resource "google_artifact_registry_repository_iam_member" "release_publisher" {
  project    = google_project.bff.project_id
  location   = google_artifact_registry_repository.images.location
  repository = google_artifact_registry_repository.images.name
  role       = "roles/artifactregistry.writer"
  member     = "serviceAccount:${google_service_account.release_publisher.email}"
}

resource "google_storage_bucket_iam_member" "release_publisher_handoff_read" {
  bucket = google_storage_bucket.release_handoffs.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.release_publisher.email}"
}

resource "google_project_iam_member" "release_logs" {
  project = google_project.bff.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.release_publisher.email}"
}

# Cloud Run runtime identity: can write and read objects in the notes bucket only.
# Holds no build, Artifact Registry push, or Cloud Run deploy permissions.
resource "google_service_account" "runtime" {
  project      = google_project.bff.project_id
  account_id   = "bff-runtime"
  display_name = "BFF Cloud Run runtime"
  depends_on   = [google_project_service.required["iam.googleapis.com"]]
}

resource "google_storage_bucket_iam_member" "runtime_notes_creator" {
  bucket = google_storage_bucket.notes.name
  role   = "roles/storage.objectCreator"
  member = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_storage_bucket_iam_member" "runtime_notes_viewer" {
  bucket = google_storage_bucket.notes.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.runtime.email}"
}

# Isolated development deployer: scoped strictly to the dev Cloud Run service and acting as runtime.
# Holds no Artifact Registry write rights, no cloudbuild.builds.create, and no production rights.
resource "google_service_account" "dev_deployer" {
  project      = google_project.bff.project_id
  account_id   = "bff-dev-deployer"
  display_name = "BFF isolated development deployer"
  depends_on   = [google_project_service.required["iam.googleapis.com"]]
}

resource "google_project_iam_member" "dev_deployer_logs" {
  project = google_project.bff.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.dev_deployer.email}"
}

resource "google_artifact_registry_repository_iam_member" "dev_deployer_image_read" {
  project    = google_project.bff.project_id
  location   = google_artifact_registry_repository.images.location
  repository = google_artifact_registry_repository.images.name
  role       = "roles/artifactregistry.reader"
  member     = "serviceAccount:${google_service_account.dev_deployer.email}"
}

resource "google_project_iam_member" "dev_deployer_run" {
  project = google_project.bff.project_id
  role    = "roles/run.developer"
  member  = "serviceAccount:${google_service_account.dev_deployer.email}"

  condition {
    title       = "bff_dev_service_only"
    description = "Restrict development deployment mutations to the single isolated dev service."
    expression  = "resource.name == 'projects/${google_project.bff.project_id}/locations/${var.region}/services/${var.dev_service_name}'"
  }
}

resource "google_service_account_iam_member" "dev_deployer_runtime_user" {
  service_account_id = "projects/${google_project.bff.project_id}/serviceAccounts/${google_service_account.runtime.account_id}@${google_project.bff.project_id}.${join(".", ["iam", "gserviceaccount", "com"])}"
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.dev_deployer.email}"
  depends_on         = [google_service_account.runtime]
}
