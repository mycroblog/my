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

variable "origin_secret" {
  description = "Value of the header that shows the server a request came through Cloudflare, for a server that checks one. THE PROVIDER HAS NO WRITE-ONLY FORM OF A RULE'S HEADER VALUE, SO IT LANDS IN THE STATE: keep the state private. Null beside ingress-cloudflare-app, which carries the one secret to this host through the rules output."
  type        = string
  default     = null
  sensitive   = true

  validation {
    condition     = var.origin_secret == null || can(regex("^[!-~]{32,}$", var.origin_secret))
    error_message = "origin_secret must be at least 32 bytes of visible ASCII with no spaces: openssl rand -hex 32."
  }
}

variable "origin_secret_id" {
  description = "Reference to the same value in the platform's secret store, when this module is what names the server's settings. Null when another module names them, such as ingress-aws-cloudfront, and this one only sends the header."
  type        = string
  default     = null
}

variable "origin_secret_header" {
  description = "Name of that header."
  type        = string
  default     = "x-my-origin-secret"
  nullable    = false

  validation {
    condition     = can(regex("^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$", var.origin_secret_header)) && !can(regex("(?i)^(cf-|x-cf-|x-forwarded-|x-amz|x-edge-|x-goog-|sec-|proxy-|if-|access-control-)", var.origin_secret_header))
    error_message = "origin_secret_header must hold letters, digits, and inner hyphens, and must not be a name that a CDN or a browser sets, such as x-forwarded-*. my config check refuses a few more, such as authorization."
  }
}

variable "tiered_cache" {
  description = "Turns on Smart Tiered Cache for the zone, which strengthens hits without changing any lifetime."
  type        = bool
  default     = true
  nullable    = false
}

locals {
  host_match = "http.host eq \"${var.host}\""
  has_secret = nonsensitive(var.origin_secret != null)

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
    # An expression, not a rule: ingress-cloudflare-app holds the secret, and a rule of its own covers this host too.
    transform = [local.host_match]
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
      condition     = var.manage_rulesets || !local.has_secret
      error_message = "A zone has one ruleset per phase, and beside ingress-cloudflare-app that one carries the one secret: leave origin_secret out here."
    }
    precondition {
      condition     = local.has_secret || var.origin_secret_id == null
      error_message = "origin_secret_id names the server's settings, so it needs origin_secret, or the server would refuse this host."
    }
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

resource "cloudflare_ruleset" "transform" {
  zone_id = var.zone_id
  name    = "my media origin secret"
  kind    = "zone"
  phase   = "http_request_late_transform"
  rules = [
    {
      description = "Show the server that the request came through Cloudflare"
      expression  = local.host_match
      action      = "rewrite"
      action_parameters = {
        headers = {
          (var.origin_secret_header) = { operation = "set", value = var.origin_secret }
        }
      }
    },
  ]

  lifecycle {
    enabled = var.manage_rulesets && local.has_secret
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
  value = merge(
    { MY_MEDIA_ORIGIN = "https://${var.host}" },
    var.origin_secret_id == null ? {} : { MY_ORIGIN_SECRET_HEADER = var.origin_secret_header },
  )
}

output "secret_env" {
  description = "The secret that the server checks, by reference, when this module names the settings."
  value       = var.origin_secret_id == null ? {} : { MY_ORIGIN_SECRET = var.origin_secret_id }
}

output "permissions" {
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
