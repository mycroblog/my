# SPDX-License-Identifier: AGPL-3.0-only

# PostgreSQL on Cloud SQL. With no network, Cloud Run reaches it through the socket of its connector; with a network, on its
# private address alone. This module has never been applied: read the README. The password is generated at apply through an ephemeral resource and goes to the user and to the URL
# secret through write-only attributes, so it reaches neither the plan nor the state.

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 7.0, < 8.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.7, < 4.0"
    }
  }
}

variable "project_id" {
  description = "Project of the instance and the secret."
  type        = string
  nullable    = false
}

variable "region" {
  description = "Region of the instance. Use the region of the service."
  type        = string
  nullable    = false
}

variable "name" {
  description = "Name of the instance. Cloud SQL does not give a name back for a week after the instance is deleted."
  type        = string
  default     = "my"
  nullable    = false
}

variable "database" {
  description = "Name of the database."
  type        = string
  default     = "my"
  nullable    = false

  validation {
    condition     = can(regex("^[a-z][a-z0-9_]*$", var.database))
    error_message = "database goes into a URL as it is: lower-case letters, digits, and underscores, starting with a letter."
  }
}

variable "username" {
  description = "Name of the user the server connects as."
  type        = string
  default     = "my"
  nullable    = false

  validation {
    condition     = can(regex("^[a-z][a-z0-9_]*$", var.username))
    error_message = "username goes into a URL as it is: lower-case letters, digits, and underscores, starting with a letter."
  }
}

variable "url_secret_id" {
  description = "ID of the secret this module creates to hold the connection URL."
  type        = string
  default     = "my-database-url"
  nullable    = false
}

variable "database_version" {
  description = "PostgreSQL version of the instance. The server is tested on 18."
  type        = string
  default     = "POSTGRES_18"
  nullable    = false
}

variable "tier" {
  description = "Machine type. The default is the smallest and costs the least; Google's SLA does not cover shared-core types."
  type        = string
  default     = "db-f1-micro"
  nullable    = false
}

variable "disk_size_gb" {
  description = "Size of the disk at creation, in GB. It grows by itself and never shrinks."
  type        = number
  default     = 10
  nullable    = false
}

variable "backups_enabled" {
  description = "Takes a daily backup. Turning it off leaves nothing to restore from."
  type        = bool
  default     = true
  nullable    = false
}

variable "point_in_time_recovery_enabled" {
  description = "Keeps the write-ahead log to restore to any moment since the oldest backup, which uses backup storage."
  type        = bool
  default     = true
  nullable    = false
}

variable "retained_backups" {
  description = "How many daily backups to keep."
  type        = number
  default     = 7
  nullable    = false
}

variable "transaction_log_retention_days" {
  description = "How many days of the write-ahead log to keep for point-in-time recovery."
  type        = number
  default     = 7
  nullable    = false
}

variable "deletion_protection" {
  description = "Refuses to delete the instance, in OpenTofu and in Google Cloud's API. prevent_destroy refuses it in any case."
  type        = bool
  default     = true
  nullable    = false
}

variable "password_version" {
  description = "Raise it to generate a new password, which goes to the user and into a new version of the URL secret in the same apply. Also the way out of an apply that stopped halfway, and what writes the URL again after url_parameters changed. Running instances keep the URL they started with: deploy again afterwards."
  type        = number
  default     = 1
  nullable    = false
}

variable "url_parameters" {
  description = "Parameters to add to the URL. Takes effect when password_version is raised, since nothing else rewrites the secret. pool_max_conns may be one of them."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "network" {
  description = "The network whose private services access gives the instance a private address, from network-gcp-vpc or an object of the same shape for a VPC of yours. With it the instance has no public address, and the app reaches it over TCP. Null, the default, keeps a public address that only the connector reaches."
  type = object({
    platform                      = string
    network                       = optional(string)
    subnetwork                    = optional(string)
    private_services_range        = optional(string)
    memorystore_connection_policy = optional(bool, false)
    nat                           = optional(bool, false)
  })
  default = null

  validation {
    condition     = var.network == null ? true : var.network.platform == "gcp"
    error_message = "network is of another platform: on Google Cloud use network-gcp-vpc, or an object of the shape in the README of deploy/opentofu."
  }

  validation {
    condition     = var.network == null ? true : can(regex("^projects/[^/]+/global/networks/[^/]+$", var.network.network))
    error_message = "network.network must be projects/<project>/global/networks/<name>."
  }

  validation {
    condition     = var.network == null ? true : var.network.private_services_range != null
    error_message = "network.private_services_range is null, and a private address on Cloud SQL needs private services access: name the range of the connection, or use network-gcp-vpc."
  }
}

locals {
  connection_name = "${var.project_id}:${var.region}:${var.name}"
  private         = var.network != null
  parameters      = merge(local.private ? { sslmode = "require" } : { host = "/cloudsql/${local.connection_name}" }, var.url_parameters)
  query           = join("&", [for k, v in local.parameters : "${k}=${v}"])
  # The URL of a Unix socket has no host: pgx takes it from the parameter. Over the socket the connector does the encryption.
  # Over the private address the server does, with sslmode=require: the instance takes encrypted connections only, and the
  # server image does not hold the instance's CA. Either way it goes to the database's own port, not to a pooler: my schema
  # apply holds a session-level lock.
  url_after_password = (
    local.private
    ? "@${google_sql_database_instance.db.private_ip_address}:5432/${var.database}?${local.query}"
    : "@/${var.database}?${local.query}"
  )
}

# Letters and digits only, so that the URL needs no escaping; 40 of them are about 238 bits.
ephemeral "random_password" "db" {
  length  = 40
  special = false
}

resource "google_sql_database_instance" "db" {
  project          = var.project_id
  region           = var.region
  name             = var.name
  database_version = var.database_version

  deletion_protection = var.deletion_protection

  settings {
    tier                        = var.tier
    edition                     = "ENTERPRISE"
    availability_type           = "ZONAL"
    disk_size                   = var.disk_size_gb
    disk_type                   = "PD_SSD"
    disk_autoresize             = true
    deletion_protection_enabled = var.deletion_protection

    backup_configuration {
      enabled                        = var.backups_enabled
      point_in_time_recovery_enabled = var.backups_enabled && var.point_in_time_recovery_enabled
      transaction_log_retention_days = var.transaction_log_retention_days

      backup_retention_settings {
        retained_backups = var.retained_backups
      }
    }

    # With no network, a public address with no network allowed: only the connector reaches it, with the identity's Cloud SQL
    # Client role. With a network, a private address alone. Google asks web applications to use this CA mode.
    ip_configuration {
      ipv4_enabled       = !local.private
      private_network    = local.private ? var.network.network : null
      allocated_ip_range = local.private ? var.network.private_services_range : null
      ssl_mode           = "ENCRYPTED_ONLY"
      server_ca_mode     = "GOOGLE_MANAGED_INTERNAL_CA"
    }
  }

  lifecycle {
    prevent_destroy = true
  }
}

resource "google_sql_database" "db" {
  project  = var.project_id
  instance = google_sql_database_instance.db.name
  name     = var.database

  # Should this resource ever leave the configuration, the data stays.
  deletion_policy = "ABANDON"

  lifecycle {
    prevent_destroy = true
  }
}

resource "google_sql_user" "server" {
  project  = var.project_id
  instance = google_sql_database_instance.db.name
  name     = var.username

  password_wo         = ephemeral.random_password.db.result
  password_wo_version = var.password_version

  # PostgreSQL refuses to drop a role that owns objects, and the server's tables are this role's.
  deletion_policy = "ABANDON"
}

resource "google_secret_manager_secret" "url" {
  project   = var.project_id
  secret_id = var.url_secret_id

  replication {
    auto {}
  }
}

# The same ephemeral value as the user's within one apply. Keep the two versions together: raising only one would leave a URL
# whose password the database does not know.
resource "google_secret_manager_secret_version" "url" {
  secret                 = google_secret_manager_secret.url.id
  secret_data_wo         = "postgres://${var.username}:${ephemeral.random_password.db.result}${local.url_after_password}"
  secret_data_wo_version = var.password_version

  # A new version replaces this one, and the app reads the latest: nothing has to delete the old one first.
  deletion_policy = "ABANDON"

  depends_on = [google_sql_user.server, google_sql_database.db]
}

output "platform" {
  description = "The platform of this module: it works only with an app of Google Cloud."
  value       = "gcp"
}

output "env" {
  description = "The settings of the database."
  value       = { MY_DATABASE = "postgres" }
}

output "secret_env" {
  description = "The connection URL, by the ID of its secret."
  value       = { MY_DATABASE_URL = var.url_secret_id }
  depends_on  = [google_secret_manager_secret_version.url]
}

output "direct_secret_env" {
  description = "Empty: the only connection is direct, so one-off work and the service use the same one."
  value       = {}
}

output "permissions" {
  description = "What the connector needs of the app's identity, which covers the project's since Cloud SQL has no grant on one instance. Nothing on a private address, which takes no connector."
  value       = [for r in [{ permission = "roles/cloudsql.client", scope = "project", resource = null }] : r if !local.private]
}

output "cloud_sql_instances" {
  description = "The instance to mount at /cloudsql on the service and the jobs. Nothing on a private address, which takes no connector."
  value       = local.private ? [] : [local.connection_name]
}

output "private_network" {
  description = "The network of the instance's private address, which must be the app's, or null."
  value       = local.private ? var.network.network : null
}

output "connection_name" {
  description = "Connection name of the instance, for the Cloud SQL Auth Proxy from your own machine. With a network, the proxy must run where the private address can be reached."
  value       = local.connection_name
}

output "url_without_password" {
  description = "The URL in the secret, with PASSWORD in place of the password."
  value       = "postgres://${var.username}:PASSWORD${local.url_after_password}"
}
