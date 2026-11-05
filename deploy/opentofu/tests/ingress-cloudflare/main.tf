# SPDX-License-Identifier: AGPL-3.0-only

# The app with both Cloudflare modules in one zone, in the place of the example's domain mapping, for the example's tests: the
# service's URL goes to the ingress while the ingress's settings go to the app, which must not be a cycle.

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.0"
    }
  }
}

locals {
  origin_host = trimprefix(module.app.service_uri, "https://")
}

module "ingress" {
  source = "../../modules/ingress-cloudflare-app"

  zone_id             = "0123456789abcdef0123456789abcdef"
  host                = "social.example.org"
  origin_host         = local.origin_host
  rewrite_host_header = true
  extra_rules         = module.media.rules
  origin_secret       = "0123456789abcdef0123456789abcdef"
  origin_secret_id    = "my-origin-secret"
}

module "media" {
  source = "../../modules/ingress-cloudflare-media"

  zone_id             = "0123456789abcdef0123456789abcdef"
  host                = "media.example.org"
  origin_host         = local.origin_host
  rewrite_host_header = true
  manage_rulesets     = false
}

module "app" {
  source = "../../modules/app-gcp-cloud-run"

  env = { MY_QUEUE = "cloudtasks" }

  project_id   = "p"
  region       = "asia-northeast1"
  image        = "ghcr.io/mycroblog/my:0.1.0"
  host         = "social.example.org"
  secret_files = { MY_KEK_FILE = "kek" }

  service_ingress = "all"

  backends = [
    { platform = module.ingress.platform, env = module.ingress.env, secret_env = module.ingress.secret_env, service_ingress = module.ingress.service_ingress, trusted_proxies = module.ingress.trusted_proxies },
    { platform = module.media.platform, env = module.media.env },
  ]
}

output "env" {
  value = module.app.env
}

output "service_ingress" {
  value = module.app.service_ingress
}
