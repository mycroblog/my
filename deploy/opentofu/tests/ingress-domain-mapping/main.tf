# SPDX-License-Identifier: AGPL-3.0-only

# The app with the domain mapping in the place of the example's load balancer, for the example's tests: the service's name
# goes to the ingress while the ingress's settings go to the app, which must not be a cycle.

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
    # Declared because the test file that runs this mocks it for every run.
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.0"
    }
  }
}

module "ingress" {
  source = "../../modules/ingress-gcp-domain-mapping"

  project_id   = "p"
  region       = "asia-northeast1"
  host         = "social.example.org"
  service_name = module.app.service_name
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
    { platform = module.ingress.platform, env = module.ingress.env, service_ingress = module.ingress.service_ingress, trusted_proxies = module.ingress.trusted_proxies },
  ]
}

output "service_ingress" {
  value = module.app.service_ingress
}

output "trusted_proxies" {
  value = module.app.env["MY_TRUSTED_PROXIES"]
}
