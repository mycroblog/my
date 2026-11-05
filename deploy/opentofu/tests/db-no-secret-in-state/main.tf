# SPDX-License-Identifier: AGPL-3.0-only

# What scripts/tofu-db-state-test.sh applies against a local stub of the Cloud SQL Admin and Secret Manager APIs, with no Google
# Cloud involved: db-gcp-cloud-sql, and a value from elsewhere, unknown at plan time, written through secrets-gcp-secret-manager
# as the output of db-neon is.

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.7"
    }
  }
}

variable "endpoint" {
  type = string
}

variable "password_version" {
  type    = number
  default = 1
}

provider "google" {
  project                         = "p"
  region                          = "r"
  access_token                    = "stub"
  secret_manager_custom_endpoint  = var.endpoint
  sql_custom_endpoint             = var.endpoint
  add_terraform_attribution_label = false
}

module "db" {
  source = "../../modules/db-gcp-cloud-sql"

  project_id       = "p"
  region           = "r"
  password_version = var.password_version
}

# Stands for what another module keeps in its own state and hands over, unknown until it exists.
resource "random_password" "elsewhere" {
  length  = 40
  special = false
}

module "secrets" {
  source = "../../modules/secrets-gcp-secret-manager"

  project_id = "p"
  referenced = { written = "written-url" }
  write_keys = ["written"]
  write_values = {
    written = random_password.elsewhere.result
  }
}
