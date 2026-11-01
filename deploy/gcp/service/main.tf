# The hosted example's service (spec 012 FR-001, FR-004, FR-008, FR-009, FR-010): one Cloud Run service that
# serves the exact image spec 009 built, by digest, as the runtime identity the infrastructure owner's contract
# names, with the server's exact origin and the bounds from the one input funnel. Every value comes from the
# validated inputs file; nothing here carries a default. This repository never applies it: an operator reads the
# plan and applies it behind the recorded gates.

locals {
  inputs = jsondecode(file(var.inputs_file))
  # The digest to serve: the release's serving digest, which the funnel has already tied to the image digest or
  # the previous one. Nothing else can be deployed, and nothing is rebuilt.
  image = "${local.inputs.contract.artifact_registry}@${local.inputs.release.serving_digest}"
}

resource "google_cloud_run_v2_service" "bff" {
  name     = "bff"
  location = local.inputs.contract.region
  project  = local.inputs.contract.project_id
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    service_account                  = local.inputs.contract.runtime_identity
    timeout                          = "${local.inputs.bounds.request_timeout_seconds}s"
    max_instance_request_concurrency = local.inputs.bounds.concurrency

    scaling {
      min_instance_count = 0
      max_instance_count = local.inputs.bounds.max_instances
    }

    containers {
      image = local.image

      ports {
        container_port = local.inputs.runtime.port
      }

      env {
        name  = "BFF_ALLOWED_ORIGIN"
        value = local.inputs.runtime.allowed_origin
      }
      env {
        name  = "BFF_VERIFIER_ISSUER"
        value = local.inputs.runtime.verifier.issuer
      }
      env {
        name  = "BFF_VERIFIER_AUDIENCE"
        value = local.inputs.runtime.verifier.audience
      }
      env {
        name  = "BFF_VERIFIER_JWKS_URL"
        value = local.inputs.runtime.verifier.jwks_url
      }
    }
  }

  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }
}

# The browser reaches the backend without a platform identity; the application verifies every call's bearer
# token itself (spec 012 FR-015), and refuses to start without a verifier.
resource "google_cloud_run_v2_service_iam_member" "public_invoker" {
  project  = google_cloud_run_v2_service.bff.project
  location = google_cloud_run_v2_service.bff.location
  name     = google_cloud_run_v2_service.bff.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

output "resources" {
  description = "The resource graph the tests pin."
  value       = ["google_cloud_run_v2_service.bff", "google_cloud_run_v2_service_iam_member.public_invoker"]
}

output "serving" {
  description = "What the plan would serve: the image reference by digest, the port and the origin."
  value = {
    image          = local.image
    port           = local.inputs.runtime.port
    allowed_origin = local.inputs.runtime.allowed_origin
    max_instances  = local.inputs.bounds.max_instances
  }
}
