# SPDX-License-Identifier: AGPL-3.0-only

# Valkey Pub/Sub as the broker of the real-time updates. It creates nothing, and uses the one Valkey setting of the server.

terraform {
  required_version = ">= 1.11"
}

# No default: beside a kv-* module, which names the URL, this is null, and two modules naming it fails the plan. Written
# out, so that leaving it out by mistake cannot pass the plan and stop the server at start.
variable "url_secret_id" {
  description = "Secret that holds the URL, as the platform's store refers to it, when no kv-* module names one. Null beside a kv-* module."
  type        = string
  nullable    = true
}

variable "tls_ca_secret_id" {
  description = "Secret that holds the PEM of a private CA, mounted as a file, with url_secret_id. A kv-* module beside this one names its own."
  type        = string
  default     = null

  validation {
    condition     = var.tls_ca_secret_id == null || var.url_secret_id != null
    error_message = "tls_ca_secret_id goes with url_secret_id: beside a kv-* module, that module names the CA."
  }
}

output "env" {
  description = "The settings of the stream."
  value       = { MY_STREAM = "valkey" }
}

output "secret_env" {
  description = "The URL by reference, unless a kv-* module names it."
  value       = var.url_secret_id == null ? {} : { MY_VALKEY_URL = var.url_secret_id }
}

output "secret_files" {
  description = "The CA, mounted as a file."
  value       = var.tls_ca_secret_id == null ? {} : { MY_VALKEY_TLS_CA_FILE = var.tls_ca_secret_id }
}

output "permissions" {
  description = "Nothing beyond reading the secrets, which the app module grants for every secret it is given."
  value       = []
}
