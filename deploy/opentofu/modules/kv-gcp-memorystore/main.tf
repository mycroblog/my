# SPDX-License-Identifier: AGPL-3.0-only

# Memorystore for Valkey, and the private network it needs. Only Private Service Connect reaches it, through a service
# connection policy that must exist first, so this module makes a VPC of its own. The server's URL takes a static password
# or none, and Memorystore's IAM authorization needs a short-lived token on each connection, so authorization stays off:
# what keeps others out is the network. The URL therefore holds no secret and goes in env.

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 7.0, < 8.0"
    }
  }
}

variable "project_id" {
  description = "Project of the instance and the network."
  type        = string
  nullable    = false
}

variable "region" {
  description = "Region of the instance and the subnet. Use the region of the app."
  type        = string
  nullable    = false
}

variable "name" {
  description = "Name of the instance, the network, the subnet, the policy, and the start of the CA secret's ID."
  type        = string
  default     = "my-valkey"
  nullable    = false

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{2,50}[a-z0-9]$", var.name))
    error_message = "name must be 4 to 52 lower-case letters, digits, and inner hyphens, starting with a letter."
  }
}

variable "subnet_cidr" {
  description = "Range of the subnet, which holds the instance's address and the addresses of Cloud Run's instances and jobs. Direct VPC egress needs a /26 or larger, and an instance takes four addresses while it runs."
  type        = string
  default     = "10.20.0.0/24"
  nullable    = false

  validation {
    condition     = can(cidrhost(var.subnet_cidr, 0)) && tonumber(split("/", var.subnet_cidr)[1]) <= 26
    error_message = "subnet_cidr must be an IPv4 range of /26 or larger."
  }
}

variable "node_type" {
  description = "Node type of the instance. The default is the smallest."
  type        = string
  default     = "SHARED_CORE_NANO"
  nullable    = false
}

variable "replica_count" {
  description = "Replicas of the shard. Each is billed as a node."
  type        = number
  default     = 0
  nullable    = false
}

variable "deletion_protection" {
  description = "Refuses to delete the instance."
  type        = bool
  default     = true
  nullable    = false
}

locals {
  # Built from the inputs so that they are known at plan time: the app module keys its secret bindings on the ID, and
  # takes its dynamic blocks from vpc_access. The outputs depend on the resources for the order.
  network    = "projects/${var.project_id}/global/networks/${var.name}"
  subnetwork = "projects/${var.project_id}/regions/${var.region}/subnetworks/${var.name}"
  ca_secret  = "${var.name}-ca"

  connections = flatten([
    for e in google_memorystore_instance.valkey.endpoints : [for c in e.connections : c.psc_auto_connection]
  ])
  # With the cluster mode off the address is the primary connection's, and a discovery connection is what the cluster
  # mode has. A reader connection only serves replicas.
  endpoint = concat(
    [for c in local.connections : c if c.connection_type == "CONNECTION_TYPE_PRIMARY"],
    [for c in local.connections : c if c.connection_type == "CONNECTION_TYPE_DISCOVERY"],
  )[0]
}

resource "google_compute_network" "valkey" {
  project                 = var.project_id
  name                    = var.name
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "valkey" {
  project       = var.project_id
  region        = var.region
  name          = var.name
  network       = google_compute_network.valkey.id
  ip_cidr_range = var.subnet_cidr
}

resource "google_network_connectivity_service_connection_policy" "valkey" {
  project       = var.project_id
  location      = var.region
  name          = var.name
  service_class = "gcp-memorystore"
  network       = google_compute_network.valkey.id

  psc_config {
    subnetworks = [google_compute_subnetwork.valkey.id]
    # Memorystore's guide: the number of instances in the region, times two.
    limit = "2"
  }
}

# One shard: the adapters keep each of their data sets in one hash slot, so more shards would hold nothing more of them.
resource "google_memorystore_instance" "valkey" {
  project                     = var.project_id
  location                    = var.region
  instance_id                 = var.name
  mode                        = "CLUSTER_DISABLED"
  shard_count                 = 1
  node_type                   = var.node_type
  replica_count               = var.replica_count
  authorization_mode          = "AUTH_DISABLED"
  transit_encryption_mode     = "SERVER_AUTHENTICATION"
  deletion_protection_enabled = var.deletion_protection

  desired_auto_created_endpoints {
    network    = google_compute_network.valkey.id
    project_id = var.project_id
  }

  depends_on = [google_network_connectivity_service_connection_policy.valkey]
}

resource "google_secret_manager_secret" "ca" {
  project   = var.project_id
  secret_id = local.ca_secret

  replication {
    auto {}
  }
}

# A secret only because the app mounts files from Secret Manager alone. It is a public certificate, so unlike a secret
# value it may reach the state, and write-only would hide a change of the CA from the next plan.
resource "google_secret_manager_secret_version" "ca" {
  secret      = google_secret_manager_secret.ca.id
  secret_data = join("\n", flatten([for m in google_memorystore_instance.valkey.managed_server_ca : [for c in m.ca_certs : c.certificates]]))
}

output "env" {
  description = "The settings of the key-value store. The URL holds no password, so it needs no secret."
  value = {
    MY_KV         = "valkey"
    MY_VALKEY_URL = "valkeys://${local.endpoint.ip_address}:${local.endpoint.port}"
  }
}

output "secret_env" {
  description = "No secrets."
  value       = {}
}

output "secret_files" {
  description = "The CA of the instance's certificate, mounted as a file."
  value       = { MY_VALKEY_TLS_CA_FILE = local.ca_secret }
  depends_on  = [google_secret_manager_secret_version.ca]
}

output "permissions" {
  description = "Nothing beyond reading the CA, which the app module grants for every secret it is given. With authorization off, no role reaches the instance."
  value       = []
}

output "vpc_access" {
  description = "What the app must attach to reach the instance: pass it to vpc_access of app-gcp-cloud-run."
  value = {
    network    = local.network
    subnetwork = local.subnetwork
  }
  depends_on = [google_compute_subnetwork.valkey]
}
