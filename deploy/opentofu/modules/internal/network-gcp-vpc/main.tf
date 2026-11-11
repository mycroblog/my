# SPDX-License-Identifier: AGPL-3.0-only

# A VPC on Google Cloud that the other modules take as network = module.network: a subnet for Cloud Run's Direct VPC egress, a
# subnet that only Private Service Connect takes addresses from, private services access for Cloud SQL, a service connection
# policy for Memorystore, and Cloud NAT as an option. It reads nothing of another module, so nothing can be circular. This
# module has never been applied: read the README.

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
  description = "Project of the network."
  type        = string
  nullable    = false
}

variable "region" {
  description = "Region of the subnets, the policy, and Cloud NAT. Use the region of the app: Direct VPC egress needs a subnet in the region of the service."
  type        = string
  nullable    = false
}

variable "name" {
  description = "Name of the network and start of the names of the rest."
  type        = string
  default     = "my"
  nullable    = false

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,40}[a-z0-9]$", var.name))
    error_message = "name must be 2 to 42 lower-case letters, digits, and inner hyphens, starting with a letter."
  }
}

variable "subnet_cidr" {
  description = "Range of the subnet of the service and the jobs. Direct VPC egress needs a /26 or larger, and takes four addresses for each instance."
  type        = string
  default     = "10.20.0.0/24"
  nullable    = false

  validation {
    condition     = can(cidrhost(var.subnet_cidr, 0)) && can(regex("^(10\\.|192\\.168\\.|172\\.(1[6-9]|2[0-9]|3[01])\\.)", var.subnet_cidr)) && tonumber(split("/", var.subnet_cidr)[1]) <= 26
    error_message = "subnet_cidr must be an RFC 1918 range of /26 or larger: with egress for private ranges only, traffic to any other range would not go through the VPC."
  }
}

variable "service_connect_cidr" {
  description = "Range of the subnet where Private Service Connect takes the addresses of its endpoints, which holds nothing else. Memorystore takes two for each instance."
  type        = string
  default     = "10.20.1.0/24"
  nullable    = false

  validation {
    condition     = can(cidrhost(var.service_connect_cidr, 0)) && can(regex("^(10\\.|192\\.168\\.|172\\.(1[6-9]|2[0-9]|3[01])\\.)", var.service_connect_cidr))
    error_message = "service_connect_cidr must be an RFC 1918 range: with egress for private ranges only, traffic to any other range would not go through the VPC."
  }
}

variable "private_services_cidr" {
  description = "Range reserved for private services access, from which Cloud SQL takes its addresses. Google's minimum is a /24 for each region or database type, so the default leaves room."
  type        = string
  default     = "10.21.0.0/20"
  nullable    = false

  validation {
    condition     = can(cidrhost(var.private_services_cidr, 0)) && can(regex("^(10\\.|192\\.168\\.|172\\.(1[6-9]|2[0-9]|3[01])\\.)", var.private_services_cidr)) && tonumber(split("/", var.private_services_cidr)[1]) <= 24
    error_message = "private_services_cidr must be an RFC 1918 range of /24 or larger: with egress for private ranges only, traffic to any other range would not go through the VPC."
  }
}

variable "nat" {
  description = "Creates Cloud NAT for the subnet of the service and the jobs, which an app with vpc_egress = ALL_TRAFFIC needs to reach the internet. Google bills it by the hour and by the GB, and a cold start through it may take 30 seconds or more."
  type        = bool
  default     = false
  nullable    = false
}

locals {
  # Built from the inputs so that they are known at plan time: the app module takes its dynamic blocks from them. The outputs
  # depend on the resources for the order.
  network    = "projects/${var.project_id}/global/networks/${var.name}"
  subnetwork = "projects/${var.project_id}/regions/${var.region}/subnetworks/${var.name}-run"
  range_name = "${var.name}-private-services"
}

resource "google_compute_network" "this" {
  project                 = var.project_id
  name                    = var.name
  auto_create_subnetworks = false
}

# Private Google Access: with vpc_egress = ALL_TRAFFIC, Google's APIs are reached through the subnet.
resource "google_compute_subnetwork" "run" {
  project                  = var.project_id
  region                   = var.region
  name                     = "${var.name}-run"
  network                  = google_compute_network.this.id
  ip_cidr_range            = var.subnet_cidr
  private_ip_google_access = true
}

# Google's advice for a subnet of a service connection policy is to hold nothing else, and Cloud Run's instances come and go.
resource "google_compute_subnetwork" "service_connect" {
  project       = var.project_id
  region        = var.region
  name          = "${var.name}-psc"
  network       = google_compute_network.this.id
  ip_cidr_range = var.service_connect_cidr
}

# Here and not in kv-gcp-memorystore: there is one policy for each network, region, and service class, whatever the number of
# instances. No limit, which Google's guide says to leave out when the number of instances is not known.
resource "google_network_connectivity_service_connection_policy" "memorystore" {
  project       = var.project_id
  location      = var.region
  name          = "${var.name}-memorystore"
  service_class = "gcp-memorystore"
  network       = google_compute_network.this.id

  psc_config {
    subnetworks = [google_compute_subnetwork.service_connect.id]
  }
}

resource "google_compute_global_address" "private_services" {
  project       = var.project_id
  name          = local.range_name
  network       = google_compute_network.this.id
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  address       = split("/", var.private_services_cidr)[0]
  prefix_length = tonumber(split("/", var.private_services_cidr)[1])
}

# Abandoned rather than deleted on destroy: Cloud SQL holds on to the range for a while after an instance goes, and the state
# then goes with the network.
resource "google_service_networking_connection" "private_services" {
  network                 = google_compute_network.this.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.private_services.name]
  deletion_policy         = "ABANDON"
}

resource "google_compute_router" "nat" {
  project = var.project_id
  region  = var.region
  name    = "${var.name}-nat"
  network = google_compute_network.this.id

  lifecycle {
    enabled = var.nat
  }
}

resource "google_compute_router_nat" "nat" {
  project                            = var.project_id
  region                             = var.region
  name                               = "${var.name}-nat"
  router                             = "${var.name}-nat"
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "LIST_OF_SUBNETWORKS"

  subnetwork {
    name                    = google_compute_subnetwork.run.id
    source_ip_ranges_to_nat = ["ALL_IP_RANGES"]
  }

  lifecycle {
    enabled = var.nat
  }

  depends_on = [google_compute_router.nat]
}

output "platform" {
  description = "The platform of this network: only a module of Google Cloud takes it."
  value       = "gcp"
}

output "network" {
  description = "The VPC, as projects/<project>/global/networks/<name>."
  value       = local.network
  depends_on  = [google_compute_network.this]
}

output "subnetwork" {
  description = "The subnet of the service and the jobs, as projects/<project>/regions/<region>/subnetworks/<name>."
  value       = local.subnetwork
  depends_on  = [google_compute_subnetwork.run]
}

output "private_services_range" {
  description = "Name of the range of private services access, with its connection made, for Cloud SQL."
  value       = local.range_name
  depends_on  = [google_service_networking_connection.private_services]
}

output "memorystore_connection_policy" {
  description = "Whether the VPC has a service connection policy of the class gcp-memorystore in this region, for Memorystore."
  value       = true
  depends_on  = [google_network_connectivity_service_connection_policy.memorystore]
}

output "nat" {
  description = "Whether the subnet of the service and the jobs reaches the internet through Cloud NAT."
  value       = var.nat
  depends_on  = [google_compute_router_nat.nat]
}
