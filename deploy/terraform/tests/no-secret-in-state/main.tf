# SPDX-License-Identifier: AGPL-3.0-only

# What scripts/terraform-state-test.sh applies against a local stub of the Secret Manager API, with no Google Cloud involved:
# the modules that generate a value.

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
  }
}

variable "endpoint" {
  type = string
}

provider "google" {
  project                         = "p"
  region                          = "r"
  access_token                    = "stub"
  secret_manager_custom_endpoint  = var.endpoint
  add_terraform_attribution_label = false
}

module "kek" {
  source = "../../modules/kek-gcp-secret-manager"

  project_id = "p"
}

module "secrets" {
  source = "../../modules/secrets-gcp-secret-manager"

  project_id         = "p"
  secrets            = { MY_VAPID_PRIVATE_KEY = "my-vapid-key" }
  generate_p256_keys = ["MY_VAPID_PRIVATE_KEY"]
}
