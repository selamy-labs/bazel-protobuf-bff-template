output "production_deployer_email" {
  description = "Service account email for the native-approved production Cloud Build trigger."
  value       = google_service_account.production_deployer.email
}

output "production_trigger_id" {
  description = "Cloud Build trigger ID for the production deployment gate."
  value       = google_cloudbuild_trigger.production.trigger_id
}

output "production_trigger_disabled" {
  description = "Whether the production Cloud Build trigger is disabled pending UUID bootstrap or release."
  value       = google_cloudbuild_trigger.production.disabled
}
