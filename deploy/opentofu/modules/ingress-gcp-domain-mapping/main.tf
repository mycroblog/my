# SPDX-License-Identifier: AGPL-3.0-only

# Traffic reaches the service directly, on a Cloud Run domain mapping of host: free, but a Preview feature that Google Cloud
# does not recommend for production.

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
  description = "Project of the service."
  type        = string
  nullable    = false
}

variable "region" {
  description = "Region of the service, which must be one that has domain mappings."
  type        = string
  nullable    = false
}

variable "host" {
  description = "Domain to map."
  type        = string
  nullable    = false
}

variable "service_name" {
  description = "Name of the Cloud Run service. Only the mapping reads it, so it may come from the app module: no output below depends on it."
  type        = string
  nullable    = false
}

resource "google_cloud_run_domain_mapping" "host" {
  project  = var.project_id
  location = var.region
  name     = var.host

  metadata {
    namespace = var.project_id
  }

  spec {
    route_name = var.service_name
  }
}

output "env" {
  description = "The peer of the server is Cloud Run's front end, and the header ends in the client's address."
  value       = { MY_TRUSTED_PROXIES = "169.254.0.0/16" }
}

output "secret_env" {
  description = "No secrets."
  value       = {}
}

output "roles" {
  description = "Nothing."
  value       = []
}

output "service_ingress" {
  description = "The mapping reaches the service as the internet does."
  value       = "INGRESS_TRAFFIC_ALL"
}

output "dns_records" {
  description = "The records to add at the DNS provider, which Cloud Run gives once the mapping exists."
  value = [
    for r in try(google_cloud_run_domain_mapping.host.status[0].resource_records, []) : {
      name   = r.name
      type   = r.type
      rrdata = r.rrdata
    }
  ]
}
