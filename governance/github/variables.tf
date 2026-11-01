# The one owner and the one name, pinned here so that no test or input can point this configuration at another
# repository (spec 010 FR-009). They are the proposed public identity (spec 001 design, section 10); the
# operator's approval of that identity is a release gate, recorded in the external packet, not here.
variable "owner" {
  description = "The one organization that owns the repository."
  type        = string
  default     = "selamy-labs"
  validation {
    condition     = var.owner == "selamy-labs"
    error_message = "This configuration declares one repository, owned by selamy-labs."
  }
}

variable "name" {
  description = "The one repository name."
  type        = string
  default     = "bazel-protobuf-bff-template"
  validation {
    condition     = var.name == "bazel-protobuf-bff-template"
    error_message = "This configuration declares one repository, bazel-protobuf-bff-template."
  }
}

variable "ruleset_active" {
  description = "The activation input (spec 010 FR-009a). False until the operator changes it and applies."
  type        = bool
  default     = false
}
