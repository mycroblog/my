# SPDX-License-Identifier: AGPL-3.0-only

# One deployment: Cloud Run, Cloud Tasks, Cloud Storage, and Pub/Sub, with a database created by hand on Neon. To change
# a backend, replace its module block: the app module takes whatever the block returns, by the contract in
# deploy/terraform/README.md.

terraform {
  required_version = ">= 1.9"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

module "db" {
  source = "../../modules/db-external"

  url_secret_id = var.database_url_secret_id
}

module "queue" {
  source = "../../modules/queue-gcp-cloud-tasks"

  project_id    = var.project_id
  region        = var.region
  host          = var.host
  tick_schedule = var.tick_schedule
}

module "blob" {
  source = "../../modules/blob-gcp-gcs"

  project_id  = var.project_id
  bucket_name = var.bucket_name
  location    = var.bucket_location
}

module "stream" {
  source = "../../modules/stream-gcp-pubsub"

  project_id = var.project_id
}

module "app" {
  source = "../../modules/app-gcp-cloud-run"

  project_id      = var.project_id
  region          = var.region
  image           = var.image
  host            = var.host
  kek_secret_id   = var.kek_secret_id
  console_enabled = var.console_enabled
  domain_mapping  = var.domain_mapping

  backends = [module.db, module.queue, module.blob, module.stream]

  env = merge(
    # Cloud Run refuses an HTTP/1 request above 32 MiB, and the server speaks HTTP/1.1. var.env can set another value.
    { MY_MEDIA_MAX_AV_BYTES = "31457280" },
    var.env,
  )
  secret_env = var.secret_env
}
