terraform {
  required_version = "= 1.13.2"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "= 8.9.0"
    }
  }
}

# The provider is bound to the contract's project and region; nothing else can be addressed.
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
