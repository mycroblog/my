# SPDX-License-Identifier: AGPL-3.0-only

# A PostgreSQL that lives outside this configuration, such as Neon. It creates nothing: the connection URLs are secrets
# the operator created by hand, so their values never reach the state.

terraform {
  required_version = ">= 1.11"
}

variable "url_secret_id" {
  description = "Secret that holds the direct connection URL, as the platform's store refers to it: on Google Cloud its ID or projects/<project>/secrets/<id>, on AWS its ARN. One-off work always uses it, since my schema apply holds a session-level lock."
  type        = string
  nullable    = false
}

variable "pooled_url_secret_id" {
  description = "Secret that holds the URL the service uses instead, such as one through a connection pooler. Null, the service uses url_secret_id too."
  type        = string
  default     = null
}

locals {
  pooled = var.pooled_url_secret_id != null
}

output "env" {
  description = "The settings of the database."
  value       = { MY_DATABASE = "postgres" }
}

output "secret_env" {
  description = "The secret settings of the service: the pooled URL when there is one, and then the direct one for LISTEN, which a pooler in transaction mode does not carry."
  value = local.pooled ? {
    MY_DATABASE_URL        = var.pooled_url_secret_id
    MY_STREAM_POSTGRES_URL = var.url_secret_id
  } : { MY_DATABASE_URL = var.url_secret_id }
}

output "direct_secret_env" {
  description = "What one-off work takes over secret_env: the direct URL. Empty when the service uses it too."
  value       = local.pooled ? { MY_DATABASE_URL = var.url_secret_id } : {}
}

output "permissions" {
  description = "Nothing beyond reading the secrets, which the app module grants for every secret it is given."
  value       = []
}
