# Mock tests of the service module against the synthetic inputs (spec 012 FR-012). They prove what the
# configuration declares, offline, with no provider call; nothing about a live project.

mock_provider "google" {}

variables {
  inputs_file = "../inputs/example.inputs.json"
}

run "serves_the_release_digest_as_the_runtime_identity" {
  command = plan

  assert {
    condition     = google_cloud_run_v2_service.bff.template[0].containers[0].image == "example-region1-docker.example.invalid/example-bff-hosted-000000/bff@sha256:0000000000000000000000000000000000000000000000000000000000000000"
    error_message = "the service must serve the release's digest from the contract's registry, nothing else"
  }
  assert {
    condition     = google_cloud_run_v2_service.bff.template[0].service_account == "bff-runtime@example.invalid"
    error_message = "the service must run as the contract's runtime identity"
  }
  assert {
    condition     = google_cloud_run_v2_service.bff.project == "example-bff-hosted-000000" && google_cloud_run_v2_service.bff.location == "example-region1"
    error_message = "the service must live in the contract's project and region"
  }
}

run "the_origin_port_and_bounds_come_from_the_funnel" {
  command = plan

  assert {
    condition     = google_cloud_run_v2_service.bff.template[0].containers[0].ports[0].container_port == 8080
    error_message = "the container port is the funnel's"
  }
  assert {
    condition     = one([for e in google_cloud_run_v2_service.bff.template[0].containers[0].env : e.value if e.name == "BFF_ALLOWED_ORIGIN"]) == "https://example-owner.github.example.invalid"
    error_message = "the allowed origin is the funnel's exact origin"
  }
  assert {
    condition     = google_cloud_run_v2_service.bff.template[0].scaling[0].max_instance_count == 2 && google_cloud_run_v2_service.bff.template[0].max_instance_request_concurrency == 20 && google_cloud_run_v2_service.bff.template[0].timeout == "30s"
    error_message = "the bounds are the funnel's"
  }
  assert {
    condition     = google_cloud_run_v2_service.bff.template[0].scaling[0].min_instance_count == 0
    error_message = "the example scales to zero"
  }
}

run "the_browser_may_call_and_the_application_verifies" {
  command = plan

  assert {
    condition     = google_cloud_run_v2_service_iam_member.public_invoker.role == "roles/run.invoker" && google_cloud_run_v2_service_iam_member.public_invoker.member == "allUsers"
    error_message = "ingress is open to the browser; the application verifies each bearer token"
  }
  assert {
    condition     = length(output.resources) == 2
    error_message = "exactly the two declared resources"
  }
}
