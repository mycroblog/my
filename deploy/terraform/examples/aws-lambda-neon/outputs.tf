# SPDX-License-Identifier: AGPL-3.0-only

output "function_url" {
  description = "URL Lambda gave the web function."
  value       = module.app.function_url
}

output "dns_records" {
  description = "What to add at the DNS provider."
  value       = module.ingress.dns_records
}

output "console" {
  description = "What console.sh reads."
  value       = module.app.console
}
