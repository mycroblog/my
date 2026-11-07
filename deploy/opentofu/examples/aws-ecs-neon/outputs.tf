# SPDX-License-Identifier: AGPL-3.0-only

output "domain_name" {
  description = "Domain name of what host points at: the distribution with cloudfront, the load balancer otherwise."
  value       = var.cloudfront ? module.cdn.domain_name : module.ingress.domain_name
}

output "dns_records" {
  description = "What to add at the DNS provider."
  value       = var.cloudfront ? module.cdn.dns_records : module.ingress.dns_records
}

output "dns_note" {
  description = "What dns_records cannot say, or null."
  value       = var.cloudfront ? module.cdn.dns_note : module.ingress.dns_note
}

output "console" {
  description = "What console.sh reads."
  value       = module.app.console
}
