# SPDX-License-Identifier: AGPL-3.0-only

output "domain_name" {
  description = "Domain name of the load balancer."
  value       = module.ingress.domain_name
}

output "dns_records" {
  description = "What to add at the DNS provider."
  value       = module.ingress.dns_records
}

output "dns_note" {
  description = "What dns_records cannot say, or null."
  value       = module.ingress.dns_note
}

output "console" {
  description = "What console.sh reads."
  value       = module.app.console
}
