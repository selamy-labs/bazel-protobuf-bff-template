output "project_id" {
  description = "Dedicated BFF project ID."
  value       = google_project.bff.project_id
}

output "region" {
  description = "Regional location for BFF resources."
  value       = var.region
}

output "artifact_registry" {
  description = "Artifact Registry image base path for the BFF container."
  value       = "${var.region}-docker.pkg.dev/${google_project.bff.project_id}/${google_artifact_registry_repository.images.repository_id}/bff"
}

output "release_publisher_email" {
  description = "Service account email for the keyless OCI image publisher."
  value       = google_service_account.release_publisher.email
}

output "dev_deployer_email" {
  description = "Service account email for the isolated development deployer."
  value       = google_service_account.dev_deployer.email
}

output "runtime_identity_email" {
  description = "Service account email for the BFF Cloud Run runtime."
  value       = google_service_account.runtime.email
}

output "notes_bucket" {
  description = "Private GCS bucket holding persisted BFF notes."
  value       = google_storage_bucket.notes.name
}

output "handoff_bucket" {
  description = "Private versioned GCS bucket holding immutable release handoff archives."
  value       = google_storage_bucket.release_handoffs.name
}

output "dev_service_name" {
  description = "Cloud Run service name scoped for bff-dev-deployer inside the dedicated dev project (matches deploy/gcp/service)."
  value       = var.dev_service_name
}

output "contract_v1" {
  description = "Versioned output contract matching deploy/gcp/inputs/contract.schema.json."
  value = {
    version           = 1
    project_id        = google_project.bff.project_id
    region            = var.region
    runtime_identity  = google_service_account.runtime.email
    build_identity    = google_service_account.dev_deployer.email
    artifact_registry = "${var.region}-docker.pkg.dev/${google_project.bff.project_id}/${google_artifact_registry_repository.images.repository_id}/bff"
    state_bucket      = var.state_bucket_name
    notes_bucket      = google_storage_bucket.notes.name
  }
}
