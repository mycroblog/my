# SPDX-License-Identifier: AGPL-3.0-only

# A Valkey, or another server that speaks the Redis protocol, that lives outside this configuration. It creates nothing:
# the URL can hold a password, so it is a secret the operator created by hand and its value never reaches the state.

terraform {
  required_version = ">= 1.11"
}

variable "url_secret_id" {
  description = "Secret that holds the URL, valkey://[user:password@]host[:port][/db] or valkeys:// for TLS, as the platform's store refers to it: on Google Cloud its ID, on AWS its ARN."
  type        = string
  nullable    = false
}

variable "tls_ca_secret_id" {
  description = "Secret that holds the PEM of the CA behind the server's TLS certificate, for a service with a private CA. Mounted as a file, so it needs a platform that mounts secrets. Null uses the system roots."
  type        = string
  default     = null
}

variable "key_prefix" {
  description = "Prefix of every key the server writes, so that several instances can share the Valkey. Null leaves the server's default, my:."
  type        = string
  default     = null

  validation {
    condition     = var.key_prefix == null || trimspace(coalesce(var.key_prefix, "x")) != ""
    error_message = "key_prefix must not be blank."
  }
}

output "env" {
  description = "The settings of the key-value store."
  value       = merge({ MY_KV = "valkey" }, var.key_prefix == null ? {} : { MY_VALKEY_KEY_PREFIX = var.key_prefix })
}

output "secret_env" {
  description = "The URL, by reference."
  value       = { MY_VALKEY_URL = var.url_secret_id }
}

output "secret_files" {
  description = "The CA, mounted as a file."
  value       = var.tls_ca_secret_id == null ? {} : { MY_VALKEY_TLS_CA_FILE = var.tls_ca_secret_id }
}

output "permissions" {
  description = "Nothing beyond reading the secrets, which the app module grants for every secret it is given."
  value       = []
}
