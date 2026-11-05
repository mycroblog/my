# SPDX-License-Identifier: AGPL-3.0-only

variable "region" {
  description = "Region of the functions, the queue, and the bucket. The image must be in ECR in this region."
  type        = string
  default     = "ap-northeast-1"
}

variable "host" {
  description = "Domain of the server. It cannot change once the server has started."
  type        = string
}

variable "image" {
  description = "Server image at creation, in ECR: a single-architecture image with the Lambda Web Adapter. The README of app-aws-lambda has how, and what it lacks."
  type        = string
}

variable "architecture" {
  description = "Architecture of the image: x86_64 or arm64."
  type        = string
  default     = "x86_64"
}

variable "certificate_arn" {
  description = "ARN of a validated certificate for host in AWS Certificate Manager, in us-east-1."
  type        = string
}

variable "origin_secret" {
  description = "Value of the header that closes the Function URL, at least 32 bytes: openssl rand -hex 32. Pass it with TF_VAR_origin_secret, and put the same value in the secret of origin_secret_name. It lands in the state, so keep the state private."
  type        = string
  sensitive   = true
}

variable "origin_secret_name" {
  description = "Secret that holds the same value, which the server reads."
  type        = string
  default     = "my-origin-secret"
}

variable "route53_zone_id" {
  description = "Public Route 53 hosted zone of host, in which the ingress creates alias records. Null leaves the records to you: the output dns_records says what to create."
  type        = string
  default     = null
}

variable "host_is_apex" {
  description = "Whether host is the apex of its DNS zone, where a plain CNAME cannot be used. It matters only without route53_zone_id."
  type        = bool
  default     = false
}

variable "database_url_secret_name" {
  description = "Secret that holds Neon's direct connection URL."
  type        = string
  default     = "my-database-url"
}

variable "vapid_secret_name" {
  description = "Secret of the VAPID key, which the first apply generates."
  type        = string
  default     = "my-vapid-key"
}

variable "kek_secret_name" {
  description = "Secret of the KEK, which the first apply generates."
  type        = string
  default     = "my-kek"
}

variable "mail_smtp_password_secret_name" {
  description = "Secret that holds the SMTP password."
  type        = string
  default     = "my-smtp-password"
}

variable "bucket_name" {
  description = "Name of the media bucket, unique across AWS."
  type        = string
}

variable "tick_schedule" {
  description = "Schedule expression of the periodic jobs. Each tick invokes the worker and touches the database, so it decides whether Neon can scale to zero."
  type        = string
  default     = "rate(1 minute)"
}

variable "mail_from" {
  description = "Sender address of emails."
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

variable "console_enabled" {
  description = "Creates the console function. Turn it on for as long as you need it."
  type        = bool
  default     = false
}

variable "env" {
  description = "Other settings by variable name."
  type        = map(string)
  default     = {}
}

variable "secret_env" {
  description = "Other secret settings: variable name to the ARN of a secret."
  type        = map(string)
  default     = {}
}
