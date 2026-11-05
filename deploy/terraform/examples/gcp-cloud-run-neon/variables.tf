# SPDX-License-Identifier: AGPL-3.0-only

variable "project_id" {
  description = "Google Cloud project."
  type        = string
}

variable "region" {
  description = "Region of the service, the jobs, and the queue."
  type        = string
  default     = "asia-northeast1"
}

variable "host" {
  description = "Domain of the server. It cannot change once the server has started."
  type        = string
}

variable "image" {
  description = "Server image at creation, as ghcr.io/mycroblog/my:<version>. After that the upgrade script changes it."
  type        = string
}

variable "database_url_secret_id" {
  description = "Secret that holds Neon's direct connection URL."
  type        = string
  default     = "my-database-url"
}

variable "kek_secret_id" {
  description = "Secret that holds the KEK."
  type        = string
  default     = "my-kek"
}

variable "bucket_name" {
  description = "Name of the media bucket, unique across Google Cloud."
  type        = string
}

variable "bucket_location" {
  description = "Location of the media bucket."
  type        = string
  default     = "asia-northeast1"
}

variable "tick_schedule" {
  description = "Cron schedule of the tick, in UTC."
  type        = string
  default     = "* * * * *"
}

variable "env" {
  description = "Other settings by variable name: mail and so on."
  type        = map(string)
  default     = {}
}

variable "secret_env" {
  description = "Other secret settings: variable name to secret ID."
  type        = map(string)
  default     = {}
}

variable "console_enabled" {
  description = "Creates the console job."
  type        = bool
  default     = false
}

variable "domain_mapping" {
  description = "Maps host to the service with a Cloud Run domain mapping."
  type        = bool
  default     = true
}

output "console" {
  description = "Pipe this to console.sh: tofu output -json console | ../../console.sh <my command>."
  value       = module.app.console
}

output "service_uri" {
  description = "URL Cloud Run gave the service."
  value       = module.app.service_uri
}

output "dns_records" {
  description = "The records to add at the DNS provider for the domain mapping."
  value       = module.app.dns_records
}
