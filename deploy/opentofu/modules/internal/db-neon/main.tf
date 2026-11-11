# SPDX-License-Identifier: AGPL-3.0-only

# PostgreSQL on Neon, with the project created by OpenTofu. This module has never been applied, and THE DATABASE PASSWORD LANDS
# IN THE STATE: the provider has no write-only attribute and no ephemeral resource, and sets the password itself. Read the
# README. db-external is the module that keeps it out.

terraform {
  required_version = ">= 1.11"

  required_providers {
    neon = {
      # Archived upstream, which says Neon will maintain it as neondatabase/neon, not yet on the OpenTofu registry. When it is,
      # this source is the one line to change. Pinned exactly: a release may replace the project, and nobody reviews it now.
      source  = "kislerdm/neon"
      version = "= 0.18.0"
    }
  }
}

variable "org_id" {
  description = "Neon organization of the project. Required: Neon warns that leaving it out can create the project in the wrong organization, and later plans may then try destructive changes."
  type        = string
  nullable    = false

  validation {
    condition     = length(var.org_id) > 0
    error_message = "org_id must name the organization."
  }
}

variable "name" {
  description = "Name of the project."
  type        = string
  default     = "my"
  nullable    = false
}

variable "region_id" {
  description = "Neon region of the project, such as aws-ap-southeast-1. It cannot change: choose the one nearest the server."
  type        = string
  nullable    = false
}

variable "pg_version" {
  description = "PostgreSQL version of the project. The server is tested on 18. It cannot change without a new project."
  type        = number
  default     = 18
  nullable    = false
}

variable "history_retention_seconds" {
  description = "How long Neon keeps the history to restore from, in seconds. The default is what the Free plan allows at most; the provider's own default of a day is above that, so this input is always sent."
  type        = number
  default     = 21600
  nullable    = false
}

variable "suspend_timeout_seconds" {
  description = "Idle seconds before the compute stops. Paid plans only: null sends nothing, which is the only value the Free plan takes, and it then stops after 5 minutes."
  type        = number
  default     = null
}

variable "url_secret_id" {
  description = "Secret to hold connection_uri, the direct connection, made with a secrets-* module, which also writes it: its ID in Secret Manager, its ARN in AWS Secrets Manager. One-off work always uses it, since my schema apply holds a session-level lock."
  type        = string
  nullable    = false
}

variable "pooled_url_secret_id" {
  description = "Secret to hold connection_uri_pooler, made and written the same way. Set, the service uses it. Null, the service uses the direct connection too."
  type        = string
  default     = null
}

resource "neon_project" "db" {
  name                      = var.name
  org_id                    = var.org_id
  region_id                 = var.region_id
  pg_version                = var.pg_version
  history_retention_seconds = var.history_retention_seconds
  suspend_timeout_seconds   = var.suspend_timeout_seconds

  # Stated because it cannot be otherwise: the provider asks Neon for the password when it creates the project and on every
  # refresh, which needs Neon to have stored it, so no would not keep the password out of the state, only break the provider.
  store_password = "yes"

  lifecycle {
    prevent_destroy = true
  }
}

locals {
  pooled = var.pooled_url_secret_id != null
}

output "platform" {
  description = "The platform of this module: it works with any app."
  value       = "neutral"
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

output "connection_uri" {
  description = "The direct connection URL, to hand to write_values of a secrets-* module. It holds the password, as the state does."
  value       = neon_project.db.connection_uri
  sensitive   = true
}

output "connection_uri_pooler" {
  description = "The connection URL through Neon's pooler in transaction mode, to hand to write_values of a secrets-* module. It holds the password, as the state does."
  value       = neon_project.db.connection_uri_pooler
  sensitive   = true
}

output "project_id" {
  description = "ID of the project."
  value       = neon_project.db.id
}
