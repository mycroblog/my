# SPDX-License-Identifier: AGPL-3.0-only

# Traffic reaches the app through Cloudflare: a proxied DNS record of host, with the cache bypassed for everything, because
# the API, ActivityPub, and HTML must never be cached. It adds no challenge: peers are not browsers. What
# stays open at the origin and what stays unverified is in the README.

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
  description = "Cloudflare zone of host."
  type        = string
  nullable    = false
}

variable "host" {
  description = "Domain of the server."
  type        = string
  nullable    = false
}

variable "origin_host" {
  description = "Host name or IPv4 address Cloudflare connects to. It may come from the app module: env depends on nothing that reads it."
  type        = string
  nullable    = false
}

variable "rewrite_host_header" {
  description = "Sends origin_host as the Host header, for an origin that routes by Host, such as *.run.app. Cloudflare offers this on the Enterprise plan only. Leave it off when the origin accepts host itself: a domain mapping of host or a load balancer."
  type        = bool
  default     = false
  nullable    = false
}

variable "proxied" {
  description = "Proxies the record through Cloudflare. A domain mapping behind it may need this off until its certificate is issued."
  type        = bool
  default     = true
  nullable    = false
}

variable "origin_secret" {
  description = "Value of the header that shows the server a request came through Cloudflare. THE PROVIDER HAS NO WRITE-ONLY FORM OF A RULE'S HEADER VALUE, SO IT LANDS IN THE STATE: keep the state private. Pass it with TF_VAR_origin_secret, and put the same value in the secret of origin_secret_id."
  type        = string
  sensitive   = true
  nullable    = false

  validation {
    condition     = can(regex("^[!-~]{32,}$", var.origin_secret))
    error_message = "origin_secret must be at least 32 bytes of visible ASCII with no spaces: openssl rand -hex 32."
  }
}

variable "origin_secret_id" {
  description = "Reference to the same value in the platform's secret store, which the server reads as MY_ORIGIN_SECRET."
  type        = string
  nullable    = false
}

variable "origin_secret_header" {
  description = "Name of that header, which nothing else uses."
  type        = string
  default     = "x-my-origin-secret"
  nullable    = false

  validation {
    condition     = can(regex("^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$", var.origin_secret_header)) && !can(regex("(?i)^(cf-|x-cf-|x-forwarded-|x-amz|x-edge-|x-goog-|sec-|proxy-|if-|access-control-)", var.origin_secret_header))
    error_message = "origin_secret_header must hold letters, digits, and inner hyphens, and must not be a name that a CDN or a browser sets, such as x-forwarded-*. my config check refuses a few more, such as authorization."
  }
}

variable "origin_secret_extra_hosts" {
  description = "Other host names of this zone that reach the server and so need the header, such as a separate account_domain. A media host beside this module comes through extra_rules instead."
  type        = list(string)
  default     = []
  nullable    = false
}

variable "extra_rules" {
  description = "The rules output of ingress-cloudflare-media in this zone. A zone has one ruleset per phase, so the two modules share them."
  type = object({
    cache    = optional(any, [])
    config   = optional(any, [])
    origin   = optional(any, [])
    firewall = optional(any, [])
    # Expressions, not rules: one rule carries the one secret to every host.
    transform = optional(any, [])
  })
  default  = {}
  nullable = false
}

locals {
  host_match = "http.host eq \"${var.host}\""

  cache_rules = concat([
    {
      description       = "Never cache the API, ActivityPub, or HTML"
      expression        = local.host_match
      action            = "set_cache_settings"
      action_parameters = { cache = false }
    },
  ], var.extra_rules.cache)

  # Strict, since Flexible would talk to the origin over HTTP, which Cloud Run answers with a redirect to itself. No Browser
  # Integrity Check, since federation peers do not send what it expects of a browser.
  config_rules = concat([
    {
      description       = "Verify the origin, and let non-browsers through"
      expression        = local.host_match
      action            = "set_config"
      action_parameters = { ssl = "strict", bic = false }
    },
  ], var.extra_rules.config)

  # Only the hosts of the server: the rest of the zone may go to origins that must not see the value.
  transform_rules = [
    {
      description = "Show the server that the request came through Cloudflare"
      expression = join(" or ", concat(
        [local.host_match],
        [for h in var.origin_secret_extra_hosts : "http.host eq \"${h}\""],
        var.extra_rules.transform,
      ))
      action = "rewrite"
      action_parameters = {
        headers = {
          (var.origin_secret_header) = { operation = "set", value = var.origin_secret }
        }
      }
    },
  ]

  origin_rules = concat(var.rewrite_host_header ? [
    {
      description       = "The origin routes by Host"
      expression        = local.host_match
      action            = "route"
      action_parameters = { host_header = var.origin_host }
    },
  ] : [], var.extra_rules.origin)
}

data "cloudflare_ip_ranges" "this" {}

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
      error_message = "A *.run.app origin routes by Host, so it needs rewrite_host_header, which Cloudflare offers on the Enterprise plan only. On other plans, use an origin that accepts host: read the README."
    }
  }
}

resource "cloudflare_ruleset" "cache" {
  zone_id = var.zone_id
  name    = "my cache"
  kind    = "zone"
  phase   = "http_request_cache_settings"
  rules   = local.cache_rules
}

resource "cloudflare_ruleset" "config" {
  zone_id = var.zone_id
  name    = "my configuration"
  kind    = "zone"
  phase   = "http_config_settings"
  rules   = local.config_rules
}

resource "cloudflare_ruleset" "origin" {
  zone_id = var.zone_id
  name    = "my origin"
  kind    = "zone"
  phase   = "http_request_origin"
  rules   = local.origin_rules

  lifecycle {
    enabled = length(local.origin_rules) > 0
  }
}

resource "cloudflare_ruleset" "transform" {
  zone_id = var.zone_id
  name    = "my origin secret"
  kind    = "zone"
  phase   = "http_request_late_transform"
  rules   = local.transform_rules
}

resource "cloudflare_ruleset" "firewall" {
  zone_id = var.zone_id
  name    = "my firewall"
  kind    = "zone"
  phase   = "http_request_firewall_custom"
  rules   = var.extra_rules.firewall

  lifecycle {
    enabled = length(var.extra_rules.firewall) > 0
  }
}

output "platform" {
  description = "The platform of this module: it works with any app."
  value       = "neutral"
}

output "env" {
  description = "The header that the proxies write, and the name of the one that shows the server a request came through Cloudflare."
  value = {
    MY_TRUSTED_PROXY_HEADER = "x-forwarded-for"
    MY_ORIGIN_SECRET_HEADER = var.origin_secret_header
  }
}

output "trusted_proxies" {
  description = "Cloudflare connects to the platform, whose own front end the app module adds. Walking X-Forwarded-For from the right past both reaches the client that connected to Cloudflare."
  value       = concat(data.cloudflare_ip_ranges.this.ipv4_cidrs, data.cloudflare_ip_ranges.this.ipv6_cidrs)
}

output "secret_env" {
  description = "The secret that the server checks, by reference."
  value       = { MY_ORIGIN_SECRET = var.origin_secret_id }
}

output "permissions" {
  description = "Nothing."
  value       = []
}

output "service_ingress" {
  description = "All: Cloudflare's traffic comes from the internet, so the platform lets everyone connect, and the server refuses what lacks the header. Read the README."
  value       = "all"
}

output "dns_records" {
  description = "None to add: this module creates the record."
  value       = []
}
