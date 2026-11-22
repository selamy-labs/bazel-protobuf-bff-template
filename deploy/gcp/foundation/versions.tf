terraform {
  required_version = "= 1.13.2"

  # Backend configuration stays outside the public repository via `-backend-config`.
  backend "gcs" {}

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "= 8.9.0"
    }
  }
}

provider "google" {
  region = var.region
}
