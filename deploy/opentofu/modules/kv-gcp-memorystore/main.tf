# SPDX-License-Identifier: AGPL-3.0-only

# Memorystore for Valkey, on a network it takes from outside. Only Private Service Connect reaches it, through a service
# connection policy of that network. The server's URL takes a static password or none, and Memorystore's IAM authorization
# needs a short-lived token on each connection, so authorization stays off: what keeps others out is the network. The URL
# therefore holds no secret and goes in env.

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
  description = "Region of the instance. Use the region of the app and of the network's policy."
  type        = string
  nullable    = false
}

variable "name" {
  description = "Name of the instance, and the start of the CA secret's ID."
  type        = string
  default     = "my-valkey"
  nullable    = false

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{2,50}[a-z0-9]$", var.name))
    error_message = "name must be 4 to 52 lower-case letters, digits, and inner hyphens, starting with a letter."
  }
}

variable "network" {
  description = "The network of the instance, from network-gcp-vpc or an object of the same shape for a VPC of yours. The app must take the same one."
  type = object({
    platform                      = string
    network                       = optional(string)
    subnetwork                    = optional(string)
    private_services_range        = optional(string)
    memorystore_connection_policy = optional(bool, false)
    nat                           = optional(bool, false)
  })
  nullable = false

  validation {
    condition     = var.network.platform == "gcp"
    error_message = "network is of another platform: on Google Cloud use network-gcp-vpc, or an object of the shape in the README of deploy/opentofu."
  }

  validation {
    condition     = can(regex("^projects/[^/]+/global/networks/[^/]+$", var.network.network))
    error_message = "network.network must be projects/<project>/global/networks/<name>."
  }

  validation {
    condition     = var.network.memorystore_connection_policy
    error_message = "network.memorystore_connection_policy is false, and Memorystore connects only through a service connection policy of the class gcp-memorystore in its region, which must exist first. Create one in your VPC and set it to true, or use network-gcp-vpc."
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
  # Built from the inputs so that it is known at plan time: the app module keys its secret bindings on the ID. The output
  # depends on the resources for the order.
  ca_secret = "${var.name}-ca"

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

  # The network's policy comes first: this reads the network's outputs, which wait for it.
  desired_auto_created_endpoints {
    network    = var.network.network
    project_id = var.project_id
  }
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

output "platform" {
  description = "The platform of this module: it works only with an app of Google Cloud."
  value       = "gcp"
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

output "private_network" {
  description = "The network of the instance, which must be the app's."
  value       = var.network.network
}
