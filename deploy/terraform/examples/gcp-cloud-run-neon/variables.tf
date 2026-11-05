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

variable "vapid_secret_id" {
  description = "Secret that holds the private key of Web Push."
  type        = string
  default     = "my-vapid-key"
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

variable "mail_from" {
  description = "Sender address of emails, as a bare address."
  type        = string
}

variable "mail_smtp_host" {
  description = "Host name of the SMTP server."
  type        = string
}

variable "mail_smtp_port" {
  description = "TCP port of the SMTP server."
  type        = number
  default     = 587
}

variable "mail_smtp_username" {
  description = "User name for SMTP AUTH."
  type        = string
}

variable "mail_smtp_password_secret_id" {
  description = "Secret that holds the SMTP password."
  type        = string
  default     = "my-smtp-password"
}

variable "tick_schedule" {
  description = "Cron schedule of the tick, in UTC."
  type        = string
  default     = "* * * * *"
}

variable "env" {
  description = "Other settings by variable name, for everything no module covers."
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

output "console" {
  description = "Pipe this to console.sh: tofu output -json console | .terraform/modules/app/deploy/terraform/console.sh <my command>."
  value       = module.app.console
}

output "service_uri" {
  description = "URL Cloud Run gave the service."
  value       = module.app.service_uri
}

output "dns_records" {
  description = "The records to add at the DNS provider."
  value       = module.ingress.dns_records
}
