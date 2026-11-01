# The one input of the service module is the path of a validated inputs file (spec 012 FR-003). The Go validator
# in deploy/gcp/inputs runs before any plan; the module reads what it accepted and adds no default of its own.
variable "inputs_file" {
  description = "The inputs file, validated by deploy/gcp/inputs before this module reads it."
  type        = string
  validation {
    condition     = can(jsondecode(file(var.inputs_file)))
    error_message = "inputs_file must name a readable JSON file."
  }
  validation {
    condition     = can(jsondecode(file(var.inputs_file)).version) && jsondecode(file(var.inputs_file)).version == 1
    error_message = "The inputs file must be schema version 1."
  }
}
