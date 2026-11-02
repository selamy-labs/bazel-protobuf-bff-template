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
  assert {
    condition     = one([for e in google_cloud_run_v2_service.bff.template[0].containers[0].env : e.value if e.name == "BFF_STORE"]) == "object" && one([for e in google_cloud_run_v2_service.bff.template[0].containers[0].env : e.value if e.name == "BFF_OBJECT_BUCKET"]) == "example-bff-hosted-000000-notes"
    error_message = "the managed store is the contract's bucket"
  }
}

# The telemetry the production binary requires (spec 011 G09): without the endpoint or the revision it refuses to
# start, and the revision fails. //deploy/gcp/inputs:inputs_test starts the real binary with this environment.
run "the_telemetry_inputs_come_from_the_funnel" {
  command = plan

  assert {
    condition     = one([for e in google_cloud_run_v2_service.bff.template[0].containers[0].env : e.value if e.name == "BFF_OTEL_ENDPOINT"]) == "https://otel.example.invalid"
    error_message = "the telemetry endpoint is the funnel's"
  }
  assert {
    condition     = one([for e in google_cloud_run_v2_service.bff.template[0].containers[0].env : e.value if e.name == "BFF_RELEASE_REVISION"]) == "0000000000000000000000000000000000000000"
    error_message = "the service reports the revision of the image it serves"
  }
  assert {
    condition     = one([for e in google_cloud_run_v2_service.bff.template[0].containers[0].env : e.value if e.name == "BFF_OTEL_ENVIRONMENT"]) == "prod" && one([for e in google_cloud_run_v2_service.bff.template[0].containers[0].env : e.value if e.name == "BFF_OTEL_SAMPLE_RATIO"]) == "1"
    error_message = "the environment and the sample ratio are the funnel's"
  }
}

# Rollback as an input (spec 012 FR-009): the same configuration with the previous digest serving. The image
# is the previous digest and the reported revision is that digest's; every other declared value is unchanged.
run "a_rollback_changes_only_the_digest" {
  command = plan

  variables {
    inputs_file = "../inputs/testdata/rollback.inputs.json"
  }

  assert {
    condition     = google_cloud_run_v2_service.bff.template[0].containers[0].image == "example-region1-docker.example.invalid/example-bff-hosted-000000/bff@sha256:2222222222222222222222222222222222222222222222222222222222222222"
    error_message = "a rollback serves the previous digest"
  }
  assert {
    condition     = one([for e in google_cloud_run_v2_service.bff.template[0].containers[0].env : e.value if e.name == "BFF_RELEASE_REVISION"]) == "2222222222222222222222222222222222222222"
    error_message = "a rollback reports the revision of the digest it serves, not the newer image's"
  }
  assert {
    condition     = google_cloud_run_v2_service.bff.template[0].service_account == "bff-runtime@example.invalid" && google_cloud_run_v2_service.bff.template[0].scaling[0].max_instance_count == 2 && google_cloud_run_v2_service.bff.template[0].max_instance_request_concurrency == 20 && google_cloud_run_v2_service.bff.template[0].timeout == "30s" && google_cloud_run_v2_service.bff.template[0].containers[0].ports[0].container_port == 8080
    error_message = "a rollback changes nothing but the digest"
  }
}

# Cancellation as an input: the plan is the same, and the pipeline's cancel step is what keeps it from being
# applied, which the offline witness checks structurally.
run "a_cancellation_leaves_the_declared_service_as_it_is" {
  command = plan

  variables {
    inputs_file = "../inputs/testdata/cancel.inputs.json"
  }

  assert {
    condition     = google_cloud_run_v2_service.bff.template[0].containers[0].image == "example-region1-docker.example.invalid/example-bff-hosted-000000/bff@sha256:0000000000000000000000000000000000000000000000000000000000000000"
    error_message = "a cancellation changes nothing in the declared service"
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
