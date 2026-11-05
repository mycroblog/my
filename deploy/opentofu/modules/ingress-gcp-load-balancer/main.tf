# SPDX-License-Identifier: AGPL-3.0-only

# Traffic reaches the service through a global external Application Load Balancer, with a Google-managed certificate for
# host and a redirect from HTTP to HTTPS. It has a monthly fee.

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
  description = "Project of the load balancer."
  type        = string
  nullable    = false
}

variable "region" {
  description = "Region of the service."
  type        = string
  nullable    = false
}

variable "host" {
  description = "Domain of the certificate."
  type        = string
  nullable    = false
}

variable "service_name" {
  description = "Name of the Cloud Run service. Only the network endpoint group reads it, so it may come from the app module: env depends on the address alone."
  type        = string
  nullable    = false
}

variable "name" {
  description = "Start of the names of the resources."
  type        = string
  default     = "my-lb"
  nullable    = false
}

# Not protected from destroy: everything here can be made again, and a new address only needs a new DNS record.
resource "google_compute_global_address" "this" {
  project = var.project_id
  name    = var.name
}

resource "google_compute_region_network_endpoint_group" "service" {
  project               = var.project_id
  region                = var.region
  name                  = var.name
  network_endpoint_type = "SERVERLESS"

  cloud_run {
    service = var.service_name
  }
}

resource "google_compute_backend_service" "service" {
  project               = var.project_id
  name                  = var.name
  load_balancing_scheme = "EXTERNAL_MANAGED"

  backend {
    group = google_compute_region_network_endpoint_group.service.id
  }
}

resource "google_compute_url_map" "https" {
  project         = var.project_id
  name            = var.name
  default_service = google_compute_backend_service.service.id
}

resource "google_compute_managed_ssl_certificate" "host" {
  project = var.project_id
  name    = var.name

  managed {
    domains = [var.host]
  }
}

resource "google_compute_target_https_proxy" "https" {
  project          = var.project_id
  name             = var.name
  url_map          = google_compute_url_map.https.id
  ssl_certificates = [google_compute_managed_ssl_certificate.host.id]
}

resource "google_compute_global_forwarding_rule" "https" {
  project               = var.project_id
  name                  = "${var.name}-https"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  ip_address            = google_compute_global_address.this.id
  port_range            = "443"
  target                = google_compute_target_https_proxy.https.id
}

resource "google_compute_url_map" "redirect" {
  project = var.project_id
  name    = "${var.name}-redirect"

  default_url_redirect {
    https_redirect         = true
    strip_query            = false
    redirect_response_code = "MOVED_PERMANENTLY_DEFAULT"
  }
}

resource "google_compute_target_http_proxy" "redirect" {
  project = var.project_id
  name    = "${var.name}-redirect"
  url_map = google_compute_url_map.redirect.id
}

resource "google_compute_global_forwarding_rule" "redirect" {
  project               = var.project_id
  name                  = "${var.name}-redirect"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  ip_address            = google_compute_global_address.this.id
  port_range            = "80"
  target                = google_compute_target_http_proxy.redirect.id
}

output "env" {
  description = "No settings."
  value       = {}
}

output "trusted_proxies" {
  description = "The load balancer ends the header with the client's address and its own, so the server must look past its own."
  value       = [google_compute_global_address.this.address]
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
  description = "Only the load balancer reaches the service: the internet cannot go around it through the run.app URL."
  value       = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"
}

output "ip_address" {
  description = "The address to point DNS at."
  value       = google_compute_global_address.this.address
}

output "dns_records" {
  description = "The records to add at the DNS provider."
  value = [
    { name = var.host, type = "A", rrdata = google_compute_global_address.this.address },
  ]
}
