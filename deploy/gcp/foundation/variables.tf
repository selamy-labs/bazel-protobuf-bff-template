variable "project_id" {
  type        = string
  description = "Reviewed globally unique dedicated BFF project ID."
  validation {
    condition     = terraform.workspace == "default" && can(regex("^[a-z][a-z0-9-]{4,28}[a-z0-9]$", var.project_id))
    error_message = "Use the default workspace and a valid 6-30 character Google Cloud project ID."
  }
}

variable "project_name" {
  type        = string
  description = "Display name for the dedicated BFF Google Cloud project."
  default     = "BFF Template Hosted"
  validation {
    condition     = length(var.project_name) >= 4 && length(var.project_name) <= 30
    error_message = "project_name must be between 4 and 30 characters."
  }
}

variable "organization_id" {
  type        = string
  description = "Verified owning GCP organization ID."
  validation {
    condition     = can(regex("^[0-9]+$", var.organization_id))
    error_message = "organization_id must be a numeric organization ID."
  }
}

variable "billing_account_id" {
  type        = string
  description = "Explicitly approved GCP billing account ID."
  validation {
    condition     = can(regex("^[A-Fa-f0-9]{6}-[A-Fa-f0-9]{6}-[A-Fa-f0-9]{6}$", var.billing_account_id))
    error_message = "billing_account_id must be a valid billing account ID."
  }
}

variable "region" {
  type        = string
  description = "Approved regional location for Artifact Registry, Cloud Build, Cloud Run, and notes bucket."
  validation {
    condition     = can(regex("^[a-z]+-[a-z]+[0-9]$", var.region))
    error_message = "region must match the contract schema regional format (for example us-central1)."
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

variable "notes_bucket_name" {
  type        = string
  description = "Private GCS bucket name storing persisted note objects."
  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9._-]{1,61}[a-z0-9]$", var.notes_bucket_name))
    error_message = "notes_bucket_name must be a valid GCS bucket name."
  }
}

variable "state_bucket_name" {
  type        = string
  description = "Operator state bucket name recorded for the versioned output contract."
  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9._-]{1,61}[a-z0-9]$", var.state_bucket_name))
    error_message = "state_bucket_name must be a valid GCS bucket name."
  }
}

variable "handoff_bucket_name" {
  type        = string
  description = "Private versioned GCS bucket holding immutable credentialless release handoff archives consumed by bff-release-publisher."
  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9._-]{1,61}[a-z0-9]$", var.handoff_bucket_name))
    error_message = "handoff_bucket_name must be a valid GCS bucket name."
  }
}

variable "dev_service_name" {
  type        = string
  description = "Isolated development Cloud Run service name inside the dedicated dev project; defaults to bff to compose directly with deploy/gcp/service."
  default     = "bff"
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,61}[a-z0-9]$", var.dev_service_name))
    error_message = "dev_service_name must be a valid Cloud Run service name."
  }
}
