# SPDX-License-Identifier: AGPL-3.0-only

# The modules that take a network on Google Cloud, sharing one, for modules/network-gcp-vpc/tests/compose.tftest.hcl: first from
# network-gcp-vpc, then from an object of the same shape for a VPC that exists already, with no network module at all.

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

locals {
  project = "p"
  region  = "asia-northeast1"

  existing_vpc = {
    platform                      = "gcp"
    network                       = "projects/p/global/networks/mine"
    subnetwork                    = "projects/p/regions/asia-northeast1/subnetworks/mine-run"
    private_services_range        = "mine-private-services"
    memorystore_connection_policy = true
    nat                           = false
  }
}

module "network" {
  source = "../../modules/network-gcp-vpc"

  project_id = local.project
  region     = local.region
}

module "db" {
  source = "../../modules/db-gcp-cloud-sql"

  project_id = local.project
  region     = local.region
  network    = module.network
}

module "kv" {
  source = "../../modules/kv-gcp-memorystore"

  project_id = local.project
  region     = local.region
  network    = module.network
}

module "app" {
  source = "../../modules/app-gcp-cloud-run"

  project_id      = local.project
  region          = local.region
  image           = "ghcr.io/mycroblog/my:0.1.0"
  host            = "social.example.org"
  service_ingress = "all"
  network         = module.network
  env             = { MY_QUEUE = "cloudtasks" }
  secret_files    = { MY_KEK_FILE = "kek" }
  backends        = [module.db, module.kv]
}

module "existing_db" {
  source = "../../modules/db-gcp-cloud-sql"

  project_id    = local.project
  region        = local.region
  name          = "mine"
  url_secret_id = "mine-database-url"
  network       = local.existing_vpc
}

module "existing_kv" {
  source = "../../modules/kv-gcp-memorystore"

  project_id = local.project
  region     = local.region
  name       = "mine-valkey"
  network    = local.existing_vpc
}

module "existing_app" {
  source = "../../modules/app-gcp-cloud-run"

  project_id      = local.project
  region          = local.region
  name            = "mine"
  image           = "ghcr.io/mycroblog/my:0.1.0"
  host            = "social.example.org"
  service_ingress = "all"
  network         = local.existing_vpc
  env             = { MY_QUEUE = "cloudtasks" }
  secret_files    = { MY_KEK_FILE = "kek" }
  backends        = [module.existing_db, module.existing_kv]
}

output "db_private_network" {
  value = module.db.private_network
}

output "kv_private_network" {
  value = module.kv.private_network
}

output "existing_db_private_network" {
  value = module.existing_db.private_network
}

output "existing_kv_private_network" {
  value = module.existing_kv.private_network
}

output "network" {
  value = module.network.network
}

output "subnetwork" {
  value = module.network.subnetwork
}

output "app_subnetwork" {
  value = module.app.subnetwork
}

output "existing_app_subnetwork" {
  value = module.existing_app.subnetwork
}

output "db_mounts" {
  value = length(module.db.cloud_sql_instances)
}
