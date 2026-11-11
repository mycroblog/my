# SPDX-License-Identifier: AGPL-3.0-only

# An app with a worker and the migration of a new image, for modules/internal/app-gcp-cloud-run/tests/upgrade.tftest.hcl. The
# migration is migrate-stub.sh, which reaches nothing, so that a test can apply one version and then another on mocked
# providers. previous_migration takes the output migration_id of an earlier run, which a test compares with this one's.

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
  }
}

variable "version_applied" {
  type = string
}

variable "previous_migration" {
  type    = string
  default = null
}

locals {
  workers = { delivery = { job_types = ["federation.fanout", "federation.deliver"] } }
}

module "queue" {
  source = "../../modules/internal/queue-gcp-cloud-tasks"

  project_id = "p"
  region     = "asia-northeast1"
  workers    = local.workers
}

module "app" {
  source = "../../modules/internal/app-gcp-cloud-run"

  project_id      = "p"
  region          = "asia-northeast1"
  image           = "ghcr.io/mycroblog/my:${var.version_applied}"
  host            = "social.example.org"
  service_ingress = "all"
  env             = { MY_STREAM = "pubsub" }
  secret_files    = { MY_KEK_FILE = "kek" }
  workers         = local.workers
  backends        = [module.queue]

  migration = {
    script  = abspath("${path.module}/migrate-stub.sh")
    version = var.version_applied
  }
}

output "migration_id" {
  value = module.app.migration_id
}

output "images" {
  value = module.app.images
}
