# SPDX-License-Identifier: AGPL-3.0-only

# A media host through Cloudflare, which caches /media/ for as long as the origin's Cache-Control says and refuses every
# other path, since the server answers whatever Host it gets. It sets no trust: media requests need no client address.

terraform {
  required_version = ">= 1.11"

  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = ">= 5.0, < 6.0"
    }
  }
}

variable "zone_id" {
  description = "Cloudflare zone of the media host."
  type        = string
  nullable    = false
}

variable "host" {
  description = "The media host."
  type        = string
  nullable    = false
}

variable "origin_host" {
  description = "Host name or IPv4 address Cloudflare connects to. It may come from the app module: env depends on nothing that reads it."
  type        = string
  nullable    = false
}

variable "rewrite_host_header" {
  description = "Sends origin_host as the Host header, for an origin that routes by Host, such as *.run.app. Cloudflare offers this on the Enterprise plan only."
  type        = bool
  default     = false
  nullable    = false
}

variable "proxied" {
  description = "Proxies the record through Cloudflare."
  type        = bool
  default     = true
  nullable    = false
}

variable "manage_rulesets" {
  description = "Creates the rulesets of the zone. A zone has one per phase, so set it to false beside ingress-cloudflare-app in the same zone, and pass the rules output to its extra_rules."
  type        = bool
  default     = true
  nullable    = false
}

variable "tiered_cache" {
  description = "Turns on Smart Tiered Cache for the zone, which strengthens hits without changing any lifetime."
  type        = bool
  default     = true
  nullable    = false
}

locals {
  host_match = "http.host eq \"${var.host}\""

  rules = {
    # Eligible includes video and audio. The default cache key already holds the query string, and with it sig, so no
    # custom key, which is for Enterprise. Both TTLs follow the origin: /media/proxy/ follows the instance's
    # remote_media_cache_ttl_seconds, which a TTL here would break.
    cache = [
      {
        description = "Cache media for as long as the origin says"
        expression  = "${local.host_match} and starts_with(http.request.uri.path, \"/media/\")"
        action      = "set_cache_settings"
        action_parameters = {
          cache       = true
          edge_ttl    = { mode = "respect_origin" }
          browser_ttl = { mode = "respect_origin" }
        }
      },
    ]
    config = [
      {
        description       = "Verify the origin"
        expression        = local.host_match
        action            = "set_config"
        action_parameters = { ssl = "strict" }
      },
    ]
    origin = var.rewrite_host_header ? [
      {
        description       = "The origin routes by Host"
        expression        = local.host_match
        action            = "route"
        action_parameters = { host_header = var.origin_host }
      },
    ] : []
    firewall = [
      {
        description = "This host serves media and nothing else"
        expression  = "${local.host_match} and not starts_with(http.request.uri.path, \"/media/\")"
        action      = "block"
      },
    ]
  }
}

resource "cloudflare_dns_record" "host" {
  zone_id = var.zone_id
  name    = var.host
  type    = can(cidrhost("${var.origin_host}/32", 0)) ? "A" : "CNAME"
  content = var.origin_host
  proxied = var.proxied
  # Automatic, as a proxied record requires.
  ttl = 1

  lifecycle {
    precondition {
      condition     = var.rewrite_host_header || !endswith(var.origin_host, ".run.app")
      error_message = "A *.run.app origin routes by Host, so it needs rewrite_host_header, which Cloudflare offers on the Enterprise plan only. On other plans, use an origin that accepts the media host: read the README."
    }
  }
}

resource "cloudflare_ruleset" "cache" {
  zone_id = var.zone_id
  name    = "my media cache"
  kind    = "zone"
  phase   = "http_request_cache_settings"
  rules   = local.rules.cache

  lifecycle {
    enabled = var.manage_rulesets
  }
}

resource "cloudflare_ruleset" "config" {
  zone_id = var.zone_id
  name    = "my media configuration"
  kind    = "zone"
  phase   = "http_config_settings"
  rules   = local.rules.config

  lifecycle {
    enabled = var.manage_rulesets
  }
}

resource "cloudflare_ruleset" "origin" {
  zone_id = var.zone_id
  name    = "my media origin"
  kind    = "zone"
  phase   = "http_request_origin"
  rules   = local.rules.origin

  lifecycle {
    enabled = var.manage_rulesets && var.rewrite_host_header
  }
}

resource "cloudflare_ruleset" "firewall" {
  zone_id = var.zone_id
  name    = "my media firewall"
  kind    = "zone"
  phase   = "http_request_firewall_custom"
  rules   = local.rules.firewall

  lifecycle {
    enabled = var.manage_rulesets
  }
}

resource "cloudflare_tiered_cache" "smart" {
  zone_id = var.zone_id
  value   = "on"

  lifecycle {
    enabled = var.tiered_cache
  }
}

output "env" {
  description = "Where the server's media URLs point. No trust: this module never names MY_TRUSTED_PROXIES, so it sits beside any other ingress."
  value       = { MY_MEDIA_ORIGIN = "https://${var.host}" }
}

output "secret_env" {
  description = "No secrets."
  value       = {}
}

output "roles" {
  description = "Nothing."
  value       = []
}

output "rules" {
  description = "The rules of this host, for extra_rules of ingress-cloudflare-app when manage_rulesets is off."
  value       = local.rules
}

output "dns_records" {
  description = "None to add: this module creates the record."
  value       = []
}
