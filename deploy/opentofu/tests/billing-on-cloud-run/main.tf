# SPDX-License-Identifier: AGPL-3.0-only

# Both billing modules beside the app of Google Cloud, for the tests of billing-aws. billing_aws takes the app's service
# account while the app takes billing_aws's settings, which is no cycle as long as backends names its outputs rather than
# the whole module, as with an ingress: a reference to the whole module waits for everything in it. Each block says which
# provider it needs.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
    google = {
      source  = "hashicorp/google"
      version = ">= 7.0, < 8.0"
    }
  }
}

variable "sources" {
  description = "Which billing modules sit in backends."
  type        = set(string)
  default     = ["aws", "gcp"]
}

variable "method" {
  type    = string
  default = "bigquery"
}

# Needs aws alone: the service account's ID arrives as a string.
module "billing_aws" {
  source = "../../modules/billing-aws"

  web_identity                     = true
  google_service_account_unique_id = module.app.service_account_unique_id
}

# Needs google.
module "billing_gcp" {
  source = "../../modules/billing-gcp"

  project_id         = "p"
  method             = var.method
  billing_account_id = var.method == "bigquery" ? "012345-6789AB-CDEF01" : null
}

# Needs google.
module "app" {
  source = "../../modules/app-gcp-cloud-run"

  project_id      = "p"
  region          = "asia-northeast1"
  image           = "ghcr.io/mycroblog/my:0.1.0"
  host            = "social.example.org"
  service_ingress = "all"
  secret_files    = { MY_KEK_FILE = "kek" }
  backends = concat(
    contains(var.sources, "aws") ? [{ env = module.billing_aws.env, permissions = module.billing_aws.permissions, billing_sources = module.billing_aws.billing_sources }] : [],
    contains(var.sources, "gcp") ? [module.billing_gcp] : [],
  )
}

output "env" {
  value = module.app.env
}
