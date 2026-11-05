# SPDX-License-Identifier: AGPL-3.0-only

# One value of workers given to a queue and an app, for modules/workers/tests/compose.tftest.hcl. It shows that nothing is circular
# and that the names of the two agree, on Cloud Tasks and on the queue in the database.

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
  }
}

locals {
  workers = {
    delivery = {
      job_types                 = ["federation.fanout", "federation.deliver"]
      max_instances             = 20
      concurrency               = 16
      max_concurrent_dispatches = 40
    }
  }
}

module "queue" {
  source = "../../modules/queue-gcp-cloud-tasks"

  project_id = "p"
  region     = "asia-northeast1"
  workers    = local.workers
}

module "app" {
  source = "../../modules/app-gcp-cloud-run"

  project_id      = "p"
  region          = "asia-northeast1"
  image           = "ghcr.io/mycroblog/my:0.1.0"
  host            = "social.example.org"
  service_ingress = "all"
  env             = { MY_STREAM = "pubsub" }
  secret_files    = { MY_KEK_FILE = "kek" }
  workers         = local.workers
  backends        = [module.queue]
}

module "queue_db" {
  source = "../../modules/queue-db"

  workers = { media = { job_types = ["federation.*"], concurrency = 3 } }
}

module "app_db" {
  source = "../../modules/app-gcp-cloud-run"

  project_id           = "p"
  region               = "asia-northeast1"
  name                 = "polls"
  image                = "ghcr.io/mycroblog/my:0.1.0"
  host                 = "social.example.org"
  service_ingress      = "all"
  min_instances        = 1
  cpu_always_allocated = true
  env                  = { MY_STREAM = "postgres" }
  secret_files         = { MY_KEK_FILE = "kek" }
  workers              = { media = { job_types = ["federation.*"], concurrency = 3 } }
  backends             = [module.queue_db]
}

output "routes" {
  value = module.queue.env["MY_QUEUE_CLOUDTASKS_ROUTES"]
}

output "worker_service" {
  value = module.app.worker_services["delivery"]
}

output "worker_routes" {
  value = module.app.worker_env["delivery"]["MY_QUEUE_CLOUDTASKS_ROUTES"]
}

output "default_routes" {
  value = module.app.env["MY_QUEUE_CLOUDTASKS_ROUTES"]
}

output "worker_db_env" {
  value = module.app_db.worker_env["media"]
}

output "default_db_env" {
  value = module.app_db.env
}
