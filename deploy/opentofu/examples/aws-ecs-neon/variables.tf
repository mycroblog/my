# SPDX-License-Identifier: AGPL-3.0-only

variable "region" {
  description = "Region of the service, the load balancer, the bucket, and SES."
  type        = string
  default     = "ap-northeast-1"
}

variable "host" {
  description = "Domain of the server. It cannot change once the server has started."
  type        = string
}

variable "availability_zones" {
  description = "Zones of the network's subnets, at least two, which the load balancer needs."
  type        = list(string)
}

variable "egress" {
  description = "How the tasks reach the internet: public-ip gives each task a public IPv4 address, nat puts them behind a NAT gateway. Both bill by the hour, so there is no default."
  type        = string
}

variable "certificate_arn" {
  description = "ARN of a validated certificate for host in AWS Certificate Manager, in this region."
  type        = string
}

variable "route53_zone_id" {
  description = "Public Route 53 hosted zone of host, in which the load balancer's records are created. Null leaves them to you: the output dns_records says what to create."
  type        = string
  default     = null
}

variable "bucket_name" {
  description = "Name of the media bucket, unique across AWS."
  type        = string
}

variable "mail_from" {
  description = "Sender address of emails, as a bare address. SES sends from its domain, which the output dns_records verifies."
  type        = string
}

variable "alert_emails" {
  description = "Addresses the alarms notify. Each must confirm the subscription."
  type        = list(string)
}

variable "console_enabled" {
  description = "Creates the console's task definition."
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
  description = "The records to add at the DNS provider: host, and the records that verify the domain of mail_from in SES."
  value       = module.mycroblog.dns_records
}

output "dns_note" {
  description = "What dns_records cannot say, or null."
  value       = module.mycroblog.dns_note
}

output "domain_name" {
  description = "Domain name of what host points at."
  value       = module.mycroblog.domain_name
}

output "version" {
  description = "The version applied, and the image the services run."
  value       = module.mycroblog.version
}

output "console" {
  description = "Pipe this to console.sh: tofu output -json console | .terraform/modules/mycroblog/deploy/opentofu/console.sh <my command>."
  value       = module.mycroblog.console
}
