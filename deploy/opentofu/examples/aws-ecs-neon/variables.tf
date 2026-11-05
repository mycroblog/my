# SPDX-License-Identifier: AGPL-3.0-only

variable "region" {
  description = "Region of the service, the load balancer, and the bucket."
  type        = string
  default     = "ap-northeast-1"
}

variable "host" {
  description = "Domain of the server. It cannot change once the server has started."
  type        = string
}

variable "image" {
  description = "Server image at creation. The released one runs as it is, from ghcr.io over the internet."
  type        = string
  default     = "ghcr.io/mycroblog/my:0.1.0"
}

variable "architecture" {
  description = "Architecture to run: x86_64 or arm64. arm64 costs less on Fargate."
  type        = string
  default     = "x86_64"
}

variable "vpc_id" {
  description = "VPC of everything here. This example creates none."
  type        = string
}

variable "load_balancer_subnet_ids" {
  description = "Public subnets of the load balancer, in at least two Availability Zones."
  type        = list(string)
}

variable "task_subnet_ids" {
  description = "Subnets of the tasks, in zones the load balancer covers. Public, with assign_public_ip; or private, with a NAT gateway of yours."
  type        = list(string)
}

variable "assign_public_ip" {
  description = "Gives each task a public IPv4 address for a public subnet, billed by the hour. False needs a NAT gateway, billed too. No default: read the costs in the README of app-aws-ecs."
  type        = bool
}

variable "cpu" {
  description = "CPU units of a task."
  type        = number
  default     = 256
}

variable "memory_mib" {
  description = "Memory of a task in MiB."
  type        = number
  default     = 1024
}

variable "desired_count" {
  description = "Tasks of the service."
  type        = number
  default     = 1
}

variable "certificate_arn" {
  description = "ARN of a validated certificate for host in AWS Certificate Manager, in this region."
  type        = string
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
  description = "Creates the console's task definition. Turn it on for as long as you need it."
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
