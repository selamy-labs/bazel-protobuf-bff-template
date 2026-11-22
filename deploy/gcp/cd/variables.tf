variable "project_id" {
  type        = string
  description = "Dedicated BFF project ID."
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{4,28}[a-z0-9]$", var.project_id))
    error_message = "project_id must be a valid Google Cloud project ID."
  }
}

variable "region" {
  type        = string
  description = "Approved Google Cloud region."
  validation {
    condition     = can(regex("^[a-z]+-[a-z]+[0-9]+$", var.region))
    error_message = "region must be a valid regional location."
  }
}

variable "registry_id" {
  type        = string
  description = "Artifact Registry Docker repository ID."
  default     = "bff-images"
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,61}[a-z0-9]$", var.registry_id))
    error_message = "registry_id must be a lowercase repository ID."
  }
}

variable "service_name" {
  type        = string
  description = "Target Cloud Run v2 service name."
  default     = "bff"
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,61}[a-z0-9]$", var.service_name))
    error_message = "Invalid Cloud Run service name."
  }
}

variable "runtime_service_account" {
  type        = string
  description = "Expected Cloud Run runtime service account email."
  validation {
    condition     = can(regex("^[^@[:space:]]+@[^@[:space:]]+$", var.runtime_service_account))
    error_message = "runtime_service_account must be a valid service account email."
  }
}

variable "notes_bucket" {
  type        = string
  description = "Expected private GCS bucket name for BFF notes persistence."
  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9._-]{1,61}[a-z0-9]$", var.notes_bucket))
    error_message = "Invalid notes bucket name."
  }
}

variable "state_bucket" {
  type        = string
  description = "Private GCS bucket storing OpenTofu remote state for the production service (must differ from notes_bucket and bundle_bucket)."
  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9._-]{1,221}[a-z0-9]$", var.state_bucket))
    error_message = "Invalid OpenTofu state bucket name."
  }
}

variable "state_prefix" {
  type        = string
  description = "Object prefix inside state_bucket dedicated exclusively to this production service state."
  default     = "bff-template/prod/service"
  validation {
    condition     = can(regex("^[A-Za-z0-9._/-]+$", var.state_prefix)) && !strcontains(var.state_prefix, "..") && !startswith(var.state_prefix, "/") && !endswith(var.state_prefix, "/")
    error_message = "state_prefix must be a clean relative object prefix without leading/trailing slashes or traversal."
  }
}

variable "release_topic" {
  type        = string
  description = "Pub/Sub topic ID triggering approved production deployments."
  validation {
    condition     = can(regex("^projects/[a-z][a-z0-9-]{4,28}[a-z0-9]/topics/[A-Za-z][A-Za-z0-9._~-]{2,254}$", var.release_topic))
    error_message = "release_topic must be a fully qualified Pub/Sub topic."
  }
}

variable "deployment_tools_image" {
  type        = string
  description = "Digest-pinned deployment tools image carrying gcloud, python3, and OpenTofu 1.13.2."
  validation {
    condition     = can(regex("^[a-z0-9.-]+(/[a-z0-9._-]+)+@sha256:[0-9a-f]{64}$", var.deployment_tools_image))
    error_message = "Deployment tools image must use an immutable digest."
  }
}

variable "production_approvers" {
  type        = set(string)
  description = "Named human IAM user principals authorized to approve native Cloud Build releases."
  validation {
    condition     = length(var.production_approvers) > 0 && alltrue([for p in var.production_approvers : can(regex("^user:[^@[:space:]]+@[^@[:space:]]+$", p))])
    error_message = "At least one individual human user approver is required."
  }
}

variable "enable_trigger" {
  type        = bool
  description = "Remains false until expected_trigger_id is recorded from the trigger UUID bootstrap."
  default     = false
}

variable "expected_trigger_id" {
  type        = string
  description = "Cloud Build trigger UUID once provisioned; required when enable_trigger is true."
  default     = ""
  validation {
    condition     = var.expected_trigger_id == "" || can(regex("^[0-9a-f-]{36}$", var.expected_trigger_id))
    error_message = "expected_trigger_id must be empty during bootstrap or an exact Cloud Build UUID."
  }
}

variable "bundle_bucket" {
  type        = string
  description = "Private GCS bucket holding the reviewed saved-plan release bundle."
  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9._-]{1,221}[a-z0-9]$", var.bundle_bucket))
    error_message = "Invalid release bundle bucket name."
  }
}

variable "bundle_object" {
  type        = string
  description = "Object path within bundle_bucket."
  validation {
    condition     = can(regex("^[A-Za-z0-9._/-]+$", var.bundle_object)) && !strcontains(var.bundle_object, "..")
    error_message = "Invalid release bundle object name."
  }
}

variable "bundle_generation" {
  type        = string
  description = "Immutable GCS object generation of the release bundle."
  validation {
    condition     = can(regex("^[1-9][0-9]*$", var.bundle_generation))
    error_message = "A specific immutable object generation is required."
  }
}

variable "bundle_sha256" {
  type        = string
  description = "Lowercase SHA-256 of the release bundle zip."
  validation {
    condition     = can(regex("^[0-9a-f]{64}$", var.bundle_sha256))
    error_message = "A lowercase bundle SHA-256 is required."
  }
}

variable "plan_sha256" {
  type        = string
  description = "Lowercase SHA-256 of the reviewed saved OpenTofu plan."
  validation {
    condition     = can(regex("^[0-9a-f]{64}$", var.plan_sha256))
    error_message = "A lowercase saved-plan SHA-256 is required."
  }
}

variable "image" {
  type        = string
  description = "Digest-pinned Artifact Registry runtime image approved for deployment."
  validation {
    condition     = can(regex("^[a-z0-9.-]+(/[a-z0-9._-]+)+@sha256:[0-9a-f]{64}$", var.image))
    error_message = "Runtime image must use an immutable digest."
  }
}

variable "expected_changes" {
  type        = map(list(string))
  description = "Exact reviewed address-to-action mutation map permitted in the saved plan."
  validation {
    condition     = length(var.expected_changes) > 0 && alltrue([for address, actions in var.expected_changes : length(address) > 0 && length(actions) == 1 && contains(["create", "update"], try(actions[0], ""))])
    error_message = "A nonempty exact address/action map containing only create or update is required."
  }
}
