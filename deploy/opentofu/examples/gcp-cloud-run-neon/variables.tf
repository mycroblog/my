# SPDX-License-Identifier: AGPL-3.0-only

variable "project_id" {
  description = "Google Cloud project."
  type        = string
}

variable "region" {
  description = "Region of the service, the jobs, the queue, and the media bucket."
  type        = string
  default     = "asia-northeast1"
}

variable "host" {
  description = "Domain of the server. It cannot change once the server has started."
  type        = string
}

variable "bucket_name" {
  description = "Name of the media bucket, unique across Google Cloud."
  type        = string
}

variable "mail_from" {
  description = "Sender address of emails, as a bare address."
  type        = string
}

variable "mail_smtp_host" {
  description = "Host name of the SMTP server."
  type        = string
}

variable "mail_smtp_username" {
  description = "User name for SMTP AUTH. Its password goes in the secret that secrets_to_fill names."
  type        = string
}

variable "console_enabled" {
  description = "Creates the console job."
  type        = bool
  default     = false
}

output "secrets_to_fill" {
  description = "The secrets to fill before the server starts, with the command that adds each."
  value       = module.mycroblog.secrets_to_fill
}

output "kek" {
  description = "Where the KEK is, and the command that copies it."
  value       = module.mycroblog.kek
}

output "dns_records" {
  description = "The records to add at the DNS provider."
  value       = module.mycroblog.dns_records
}

output "service_uri" {
  description = "URL Cloud Run gave the service."
  value       = module.mycroblog.service_uri
}

output "version" {
  description = "The version applied, and the image the services run."
  value       = module.mycroblog.version
}

output "console" {
  description = "Pipe this to console.sh: tofu output -json console | .terraform/modules/mycroblog/deploy/opentofu/console.sh <my command>."
  value       = module.mycroblog.console
}

variable "alert_emails" {
  description = "Addresses the alerts notify."
  type        = list(string)
}
